package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

// Adapter hides the one GORM session and database/sql pool used by Admin.
type Adapter struct {
	database string
	gorm     *gorm.DB
	pool     *sql.DB
}

func Open(ctx context.Context, settings config.MySQL) (*Adapter, error) {
	adapter, err := OpenMaintenance(ctx, settings)
	if err != nil {
		return nil, err
	}
	if err := adapter.Ready(ctx); err != nil {
		_ = adapter.Close()
		return nil, fmt.Errorf("Managed Data Source is unavailable: %w", err)
	}
	return adapter, nil
}

// OpenMaintenance opens only the configured database connection. Maintenance
// tools must work before normal Admin's required control schemas are installed.
func OpenMaintenance(ctx context.Context, settings config.MySQL) (*Adapter, error) {
	driverConfig := driver.Config{
		User:              settings.User,
		Passwd:            settings.Password,
		Net:               settings.Network,
		Addr:              settings.Address,
		DBName:            settings.Database,
		TLSConfig:         settings.TLSMode,
		ParseTime:         true,
		Loc:               sqlUTC,
		ClientFoundRows:   true,
		AllowAllFiles:     false,
		InterpolateParams: false,
		MultiStatements:   false,
		Timeout:           settings.ConnectTimeout,
		ReadTimeout:       settings.ReadTimeout,
		WriteTimeout:      settings.WriteTimeout,
		Collation:         "utf8mb4_0900_ai_ci",
		Params:            map[string]string{"time_zone": "'+00:00'"},
	}
	dsn := driverConfig.FormatDSN()

	database, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       dsn,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("Managed Data Source is unavailable: %w", err)
	}

	pool, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("access Managed Data Source pool: %w", err)
	}
	pool.SetMaxOpenConns(settings.MaxOpenConnections)
	pool.SetMaxIdleConns(settings.MaxIdleConnections)
	pool.SetConnMaxLifetime(settings.ConnectionMaxLife)
	pool.SetConnMaxIdleTime(settings.ConnectionMaxIdle)

	adapter := &Adapter{database: settings.Database, gorm: database, pool: pool}
	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("Managed Data Source is unavailable: %w", err)
	}
	return adapter, nil
}

func (adapter *Adapter) Close() error {
	return adapter.pool.Close()
}

func (adapter *Adapter) Ready(ctx context.Context) error {
	if err := adapter.pool.PingContext(ctx); err != nil {
		return fmt.Errorf("ping MySQL: %w", err)
	}
	for _, table := range []string{"rcc_table_policies", "rcc_query_policies", "rcc_mutation_policies"} {
		rows, err := adapter.gorm.WithContext(ctx).Raw("SELECT 1 FROM `" + table + "` LIMIT 0").Rows()
		if err != nil {
			return fmt.Errorf("Policy Catalog unavailable: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("Policy Catalog unavailable: %w", err)
		}
	}
	if err := adapter.accountSchemaReady(ctx); err != nil {
		return err
	}
	if err := adapter.accountRoleSchemaReady(ctx); err != nil {
		return err
	}
	if err := adapter.recordVersionSchemaReady(ctx); err != nil {
		return err
	}
	return adapter.releaseSchemaReady(ctx)
}

func (adapter *Adapter) ListDatabaseTables(ctx context.Context) ([]domain.DatabaseTable, error) {
	metadata, err := adapter.readTableMetadata(ctx)
	if err != nil {
		return nil, err
	}

	tables := make([]domain.DatabaseTable, 0, len(metadata))
	for _, item := range metadata {
		tables = append(tables, domain.DescribeDatabaseTable(
			item.name,
			item.comment,
			item.primaryKey,
			item.policyExists,
			item.policyEnabled,
		))
	}
	sort.Slice(tables, func(left, right int) bool {
		return tables[left].Name < tables[right].Name
	})
	return tables, nil
}

func (adapter *Adapter) GetDatabaseTable(ctx context.Context, tableName string) (domain.DatabaseTable, error) {
	tables, err := adapter.ListDatabaseTables(ctx)
	if err != nil {
		return domain.DatabaseTable{}, err
	}
	for _, table := range tables {
		if table.Name == tableName {
			return table, nil
		}
	}
	return domain.DatabaseTable{}, application.ErrDatabaseTableNotFound
}

type schemaTableRow struct {
	TableType string `gorm:"column:table_type"`
}

type schemaColumnRow struct {
	TextCapacity         sql.NullInt64  `gorm:"column:text_capacity"`
	Name                 string         `gorm:"column:column_name"`
	DataType             string         `gorm:"column:data_type"`
	ColumnType           string         `gorm:"column:column_type"`
	Nullable             string         `gorm:"column:is_nullable"`
	ColumnKey            string         `gorm:"column:column_key"`
	DefaultValue         sql.NullString `gorm:"column:column_default"`
	Extra                string         `gorm:"column:extra"`
	GenerationExpression string         `gorm:"column:generation_expression"`
}

func (adapter *Adapter) GetTableSchema(ctx context.Context, tableName string) (domain.TableSchema, error) {
	return adapter.getTableSchema(ctx, adapter.gorm, tableName)
}

func (adapter *Adapter) getTableSchema(ctx context.Context, database *gorm.DB, tableName string) (domain.TableSchema, error) {
	if protectedTable(tableName) {
		return domain.TableSchema{}, application.ErrProtectedTable
	}
	var table schemaTableRow
	result := database.WithContext(ctx).Raw(`
SELECT TABLE_TYPE AS table_type
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, adapter.database, tableName).Scan(&table)
	if result.Error != nil {
		return domain.TableSchema{}, fmt.Errorf("read live table: %w", result.Error)
	}
	if result.RowsAffected == 0 || table.TableType != "BASE TABLE" {
		return domain.TableSchema{}, application.ErrDatabaseTableNotFound
	}

	var rows []schemaColumnRow
	if err := database.WithContext(ctx).Raw(`
SELECT
  COLUMN_NAME AS column_name,
  DATA_TYPE AS data_type,
  COLUMN_TYPE AS column_type,
  CHARACTER_MAXIMUM_LENGTH AS text_capacity,
  IS_NULLABLE AS is_nullable,
  COLUMN_KEY AS column_key,
  COLUMN_DEFAULT AS column_default,
  EXTRA AS extra,
  GENERATION_EXPRESSION AS generation_expression
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION`, adapter.database, tableName).Scan(&rows).Error; err != nil {
		return domain.TableSchema{}, fmt.Errorf("read live table columns: %w", err)
	}

	columns := make([]domain.Column, 0, len(rows))
	primaryKey := make([]string, 0, 1)
	for _, row := range rows {
		// DEFAULT_GENERATED marks an expression default, not a computed column.
		extra := strings.ToLower(row.Extra)
		floatBits := 0
		if row.DataType == "float" {
			floatBits = 32
		}
		columns = append(columns, domain.Column{
			Name:          row.Name,
			TextCapacity:  liveTextCapacity(row.DataType, row.TextCapacity),
			FloatBits:     floatBits,
			Type:          liveColumnType(row.DataType, row.ColumnType),
			Nullable:      row.Nullable == "YES",
			Generated:     row.GenerationExpression != "" || strings.Contains(extra, "stored generated") || strings.Contains(extra, "virtual generated"),
			AutoIncrement: strings.Contains(extra, "auto_increment"),
			HasDefault:    row.DefaultValue.Valid || strings.Contains(extra, "default_generated"),
		})
		if row.ColumnKey == "PRI" {
			primaryKey = append(primaryKey, row.Name)
		}
	}
	description := domain.DescribeDatabaseTable(tableName, "", primaryKey, false, false)
	return domain.TableSchema{
		Name:                  tableName,
		Columns:               columns,
		Compatible:            description.Compatible,
		IncompatibilityReason: description.IncompatibilityReason,
	}, nil
}

func liveColumnType(dataType, columnType string) domain.ColumnType {
	typeName := strings.ToLower(dataType)
	definition := strings.ToLower(columnType)
	switch typeName {
	case "tinyint":
		if strings.HasPrefix(definition, "tinyint(1)") {
			return domain.ColumnTypeBoolean
		}
		if strings.Contains(definition, "unsigned") {
			return domain.ColumnTypeUInt64
		}
		return domain.ColumnTypeInt64
	case "smallint", "mediumint", "int", "integer", "bigint":
		if strings.Contains(definition, "unsigned") {
			return domain.ColumnTypeUInt64
		}
		return domain.ColumnTypeInt64
	case "decimal", "numeric":
		return domain.ColumnTypeDecimal
	case "float", "double", "real":
		return domain.ColumnTypeFloat64
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum":
		return domain.ColumnTypeString
	case "date":
		return domain.ColumnTypeDate
	case "time":
		return domain.ColumnTypeTime
	case "datetime":
		return domain.ColumnTypeDateTime
	case "timestamp":
		return domain.ColumnTypeTimestamp
	case "json":
		return domain.ColumnTypeJSON
	default:
		return domain.ColumnTypeUnsupported
	}
}

type policyRecord struct {
	ID                 uint64    `gorm:"column:id;primaryKey"`
	Table              string    `gorm:"column:table_name"`
	QueryPolicyCode    string    `gorm:"column:query_policy_code"`
	MutationPolicyCode string    `gorm:"column:mutation_policy_code"`
	Enabled            bool      `gorm:"column:enabled"`
	Creator            string    `gorm:"column:creator"`
	Modifier           string    `gorm:"column:modifier"`
	CreatedAt          time.Time `gorm:"column:gmt_created"`
	UpdatedAt          time.Time `gorm:"column:gmt_modified"`
}

func (policyRecord) TableName() string {
	return "rcc_table_policies"
}

func (adapter *Adapter) Create(ctx context.Context, policy domain.TablePolicy, operator string) error {
	record := policyRecord{
		Table: policy.TableName, QueryPolicyCode: policy.QueryPolicyCode,
		MutationPolicyCode: policy.MutationPolicyCode, Enabled: false,
		Creator: operator, Modifier: operator,
	}
	if err := adapter.gorm.WithContext(ctx).Create(&record).Error; err != nil {
		var mysqlError *driver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return domain.ErrTablePolicyExists
		}
		return fmt.Errorf("create Table Policy: %w", err)
	}
	return nil
}

// CreateWithActivePolicyCodes locks both selected definitions and rechecks
// lifecycle state in the same transaction as the assignment write. This closes
// the race between application validation and concurrent deprecation.
func (adapter *Adapter) CreateWithActivePolicyCodes(ctx context.Context, policy domain.TablePolicy, operator string) error {
	return adapter.gorm.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := activeAssignmentDefinitions(transaction, policy.QueryPolicyCode, policy.MutationPolicyCode); err != nil {
			return err
		}
		record := policyRecord{
			Table: policy.TableName, QueryPolicyCode: policy.QueryPolicyCode,
			MutationPolicyCode: policy.MutationPolicyCode, Enabled: false,
			Creator: operator, Modifier: operator,
		}
		if err := transaction.Create(&record).Error; err != nil {
			var mysqlError *driver.MySQLError
			if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
				return domain.ErrTablePolicyExists
			}
			return fmt.Errorf("create Table Policy: %w", err)
		}
		return nil
	})
}

func (adapter *Adapter) List(ctx context.Context) ([]domain.TablePolicy, error) {
	var records []policyRecord
	if err := adapter.gorm.WithContext(ctx).Order("table_name ASC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list Table Policies: %w", err)
	}
	policies := make([]domain.TablePolicy, 0, len(records))
	for _, record := range records {
		policies = append(policies, record.policy())
	}
	return policies, nil
}

func (adapter *Adapter) Get(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	return adapter.getTablePolicy(ctx, adapter.gorm, tableName)
}

func (adapter *Adapter) getTablePolicy(ctx context.Context, database *gorm.DB, tableName string) (domain.TablePolicy, error) {
	var record policyRecord
	err := database.WithContext(ctx).Where("table_name = ?", tableName).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	if err != nil {
		return domain.TablePolicy{}, fmt.Errorf("get Table Policy: %w", err)
	}
	return record.policy(), nil
}

func (adapter *Adapter) Replace(ctx context.Context, policy domain.TablePolicy, operator string) (domain.TablePolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&policyRecord{}).
		Where("table_name = ?", policy.TableName).
		Updates(map[string]any{
			"query_policy_code":    policy.QueryPolicyCode,
			"mutation_policy_code": policy.MutationPolicyCode,
			"modifier":             operator,
		})
	if result.Error != nil {
		return domain.TablePolicy{}, fmt.Errorf("replace Table Policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	return adapter.Get(ctx, policy.TableName)
}

func (adapter *Adapter) ReplaceWithActivePolicyCodes(ctx context.Context, policy domain.TablePolicy, operator string) (domain.TablePolicy, error) {
	err := adapter.gorm.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := activeAssignmentDefinitions(transaction, policy.QueryPolicyCode, policy.MutationPolicyCode); err != nil {
			return err
		}
		result := transaction.Model(&policyRecord{}).Where("table_name = ?", policy.TableName).Updates(map[string]any{
			"query_policy_code": policy.QueryPolicyCode, "mutation_policy_code": policy.MutationPolicyCode,
			"modifier": operator,
		})
		if result.Error != nil {
			return fmt.Errorf("replace Table Policy: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return domain.ErrTablePolicyNotFound
		}
		return nil
	})
	if err != nil {
		return domain.TablePolicy{}, err
	}
	return adapter.Get(ctx, policy.TableName)
}

func activeAssignmentDefinitions(transaction *gorm.DB, queryCode, mutationCode string) error {
	var query queryPolicyRecord
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code = ?", queryCode).First(&query).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrQueryPolicyNotAssignable
	} else if err != nil {
		return fmt.Errorf("validate Query Policy assignment: %w", err)
	}
	if query.Status != domain.PolicyStatusActive {
		return application.ErrQueryPolicyNotAssignable
	}
	var mutation mutationPolicyRecord
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code = ?", mutationCode).First(&mutation).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrMutationPolicyNotAssignable
	} else if err != nil {
		return fmt.Errorf("validate Mutation Policy assignment: %w", err)
	}
	if mutation.Status != domain.PolicyStatusActive {
		return application.ErrMutationPolicyNotAssignable
	}
	return nil
}

func (adapter *Adapter) SetEnabled(ctx context.Context, tableName string, enabled bool, operator string) (domain.TablePolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&policyRecord{}).
		Where("table_name = ?", tableName).
		Updates(map[string]any{"enabled": enabled, "modifier": operator})
	if result.Error != nil {
		return domain.TablePolicy{}, fmt.Errorf("set Table Policy state: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	return adapter.Get(ctx, tableName)
}

func (record policyRecord) policy() domain.TablePolicy {
	return domain.TablePolicy{
		QueryPolicyCode: record.QueryPolicyCode, MutationPolicyCode: record.MutationPolicyCode,
		TableName: record.Table, Enabled: record.Enabled, Creator: record.Creator,
		Modifier: record.Modifier, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

type queryPolicyRecord struct {
	ID                    uint64              `gorm:"column:id;primaryKey"`
	Code                  string              `gorm:"column:code"`
	Name                  string              `gorm:"column:name"`
	Description           string              `gorm:"column:description"`
	TypeCode              string              `gorm:"column:type_code"`
	DefaultOrderField     string              `gorm:"column:default_order_field"`
	DefaultOrderDirection string              `gorm:"column:default_order_direction"`
	DefaultPageSize       int                 `gorm:"column:default_page_size"`
	MaxPageSize           int                 `gorm:"column:max_page_size"`
	Status                domain.PolicyStatus `gorm:"column:status"`
	Creator               string              `gorm:"column:creator"`
	Modifier              string              `gorm:"column:modifier"`
	CreatedAt             time.Time           `gorm:"column:gmt_created"`
	UpdatedAt             time.Time           `gorm:"column:gmt_modified"`
}

func (queryPolicyRecord) TableName() string { return "rcc_query_policies" }

func (adapter *Adapter) CreateQueryPolicy(ctx context.Context, policy domain.QueryPolicy, operator string) (domain.QueryPolicy, error) {
	record := queryPolicyRecord{
		Code: policy.Code, Name: policy.Name, Description: policy.Description,
		TypeCode: policy.TypeCode, DefaultOrderField: policy.DefaultOrderField,
		DefaultOrderDirection: policy.DefaultOrderDirection, DefaultPageSize: policy.DefaultPageSize,
		MaxPageSize: policy.MaxPageSize, Status: domain.PolicyStatusDraft,
		Creator: operator, Modifier: operator,
	}
	if err := adapter.gorm.WithContext(ctx).Create(&record).Error; err != nil {
		var mysqlError *driver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return domain.QueryPolicy{}, domain.ErrQueryPolicyExists
		}
		return domain.QueryPolicy{}, fmt.Errorf("create Query Policy: %w", err)
	}
	return record.policy(), nil
}

func (adapter *Adapter) ListQueryPolicies(ctx context.Context) ([]domain.QueryPolicy, error) {
	var records []queryPolicyRecord
	if err := adapter.gorm.WithContext(ctx).Order("code ASC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list Query Policies: %w", err)
	}
	policies := make([]domain.QueryPolicy, 0, len(records))
	for _, record := range records {
		policies = append(policies, record.policy())
	}
	return policies, nil
}

func (adapter *Adapter) GetQueryPolicy(ctx context.Context, code string) (domain.QueryPolicy, error) {
	return adapter.getQueryPolicy(ctx, adapter.gorm, code)
}

func (adapter *Adapter) getQueryPolicy(ctx context.Context, database *gorm.DB, code string) (domain.QueryPolicy, error) {
	var record queryPolicyRecord
	err := database.WithContext(ctx).Where("code = ?", code).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyNotFound
	}
	if err != nil {
		return domain.QueryPolicy{}, fmt.Errorf("get Query Policy: %w", err)
	}
	return record.policy(), nil
}

func (adapter *Adapter) ReplaceDraftQueryPolicy(ctx context.Context, policy domain.QueryPolicy, operator string) (domain.QueryPolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&queryPolicyRecord{}).
		Where("code = ? AND status = ?", policy.Code, domain.PolicyStatusDraft).
		Updates(map[string]any{
			"name": policy.Name, "description": policy.Description, "type_code": policy.TypeCode,
			"default_order_field": policy.DefaultOrderField, "default_order_direction": policy.DefaultOrderDirection,
			"default_page_size": policy.DefaultPageSize, "max_page_size": policy.MaxPageSize, "modifier": operator,
		})
	if result.Error != nil {
		return domain.QueryPolicy{}, fmt.Errorf("replace Draft Query Policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.QueryPolicy{}, adapter.queryPolicyWriteConflict(ctx, policy.Code)
	}
	return adapter.GetQueryPolicy(ctx, policy.Code)
}

func (adapter *Adapter) SetQueryPolicyStatus(ctx context.Context, code string, from, to domain.PolicyStatus, operator string) (domain.QueryPolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&queryPolicyRecord{}).
		Where("code = ? AND status = ?", code, from).
		Updates(map[string]any{"status": to, "modifier": operator})
	if result.Error != nil {
		return domain.QueryPolicy{}, fmt.Errorf("transition Query Policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.QueryPolicy{}, adapter.queryPolicyWriteConflict(ctx, code)
	}
	return adapter.GetQueryPolicy(ctx, code)
}

func (adapter *Adapter) UpdateQueryPolicyMetadata(ctx context.Context, code, name, description, operator string) (domain.QueryPolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&queryPolicyRecord{}).
		Where("code = ? AND status IN ?", code, []domain.PolicyStatus{domain.PolicyStatusActive, domain.PolicyStatusDeprecated}).
		Updates(map[string]any{"name": name, "description": description, "modifier": operator})
	if result.Error != nil {
		return domain.QueryPolicy{}, fmt.Errorf("update Query Policy metadata: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.QueryPolicy{}, adapter.queryPolicyWriteConflict(ctx, code)
	}
	return adapter.GetQueryPolicy(ctx, code)
}

func (adapter *Adapter) DeleteDraftQueryPolicy(ctx context.Context, code string) error {
	result := adapter.gorm.WithContext(ctx).Where("code = ? AND status = ?", code, domain.PolicyStatusDraft).Delete(&queryPolicyRecord{})
	if result.Error != nil {
		return fmt.Errorf("delete Draft Query Policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return adapter.queryPolicyWriteConflict(ctx, code)
	}
	return nil
}

func (adapter *Adapter) queryPolicyWriteConflict(ctx context.Context, code string) error {
	if _, err := adapter.GetQueryPolicy(ctx, code); err != nil {
		return err
	}
	return domain.ErrQueryPolicyStateConflict
}

func (record queryPolicyRecord) policy() domain.QueryPolicy {
	return domain.QueryPolicy{
		Code: record.Code, Name: record.Name, Description: record.Description, TypeCode: record.TypeCode,
		DefaultOrderField: record.DefaultOrderField, DefaultOrderDirection: record.DefaultOrderDirection,
		DefaultPageSize: record.DefaultPageSize, MaxPageSize: record.MaxPageSize, Status: record.Status,
		Creator: record.Creator, Modifier: record.Modifier, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

type mutationPolicyRecord struct {
	ID                  uint64              `gorm:"column:id;primaryKey"`
	Code                string              `gorm:"column:code"`
	Name                string              `gorm:"column:name"`
	Description         string              `gorm:"column:description"`
	TypeCode            string              `gorm:"column:type_code"`
	AllowAdd            bool                `gorm:"column:allow_add"`
	AllowModify         bool                `gorm:"column:allow_modify"`
	AllowDelete         bool                `gorm:"column:allow_delete"`
	CreateOperatorField *string             `gorm:"column:create_operator_field"`
	CreateTimeField     *string             `gorm:"column:create_time_field"`
	ModifyOperatorField *string             `gorm:"column:modify_operator_field"`
	ModifyTimeField     *string             `gorm:"column:modify_time_field"`
	Status              domain.PolicyStatus `gorm:"column:status"`
	Creator             string              `gorm:"column:creator"`
	Modifier            string              `gorm:"column:modifier"`
	CreatedAt           time.Time           `gorm:"column:gmt_created"`
	UpdatedAt           time.Time           `gorm:"column:gmt_modified"`
}

func (mutationPolicyRecord) TableName() string { return "rcc_mutation_policies" }

func (adapter *Adapter) CreateMutationPolicy(ctx context.Context, policy domain.MutationPolicy, operator string) (domain.MutationPolicy, error) {
	record := mutationPolicyRecord{
		Code: policy.Code, Name: policy.Name, Description: policy.Description, TypeCode: policy.TypeCode,
		AllowAdd: policy.AllowAdd, AllowModify: policy.AllowModify, AllowDelete: policy.AllowDelete,
		CreateOperatorField: policy.CreateOperatorField, CreateTimeField: policy.CreateTimeField,
		ModifyOperatorField: policy.ModifyOperatorField, ModifyTimeField: policy.ModifyTimeField,
		Status: domain.PolicyStatusDraft, Creator: operator, Modifier: operator,
	}
	if err := adapter.gorm.WithContext(ctx).Create(&record).Error; err != nil {
		var mysqlError *driver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return domain.MutationPolicy{}, domain.ErrMutationPolicyExists
		}
		return domain.MutationPolicy{}, fmt.Errorf("create Mutation Policy: %w", err)
	}
	return record.policy(), nil
}

func (adapter *Adapter) ListMutationPolicies(ctx context.Context) ([]domain.MutationPolicy, error) {
	var records []mutationPolicyRecord
	if err := adapter.gorm.WithContext(ctx).Order("code ASC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list Mutation Policies: %w", err)
	}
	policies := make([]domain.MutationPolicy, 0, len(records))
	for _, record := range records {
		policies = append(policies, record.policy())
	}
	return policies, nil
}

func (adapter *Adapter) GetMutationPolicy(ctx context.Context, code string) (domain.MutationPolicy, error) {
	return adapter.getMutationPolicy(ctx, adapter.gorm, code)
}

func (adapter *Adapter) getMutationPolicy(ctx context.Context, database *gorm.DB, code string) (domain.MutationPolicy, error) {
	var record mutationPolicyRecord
	err := database.WithContext(ctx).Where("code = ?", code).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyNotFound
	}
	if err != nil {
		return domain.MutationPolicy{}, fmt.Errorf("get Mutation Policy: %w", err)
	}
	return record.policy(), nil
}

func (adapter *Adapter) ReplaceDraftMutationPolicy(ctx context.Context, policy domain.MutationPolicy, operator string) (domain.MutationPolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&mutationPolicyRecord{}).
		Where("code = ? AND status = ?", policy.Code, domain.PolicyStatusDraft).
		Updates(map[string]any{
			"name": policy.Name, "description": policy.Description, "type_code": policy.TypeCode,
			"allow_add": policy.AllowAdd, "allow_modify": policy.AllowModify, "allow_delete": policy.AllowDelete,
			"create_operator_field": policy.CreateOperatorField, "create_time_field": policy.CreateTimeField,
			"modify_operator_field": policy.ModifyOperatorField, "modify_time_field": policy.ModifyTimeField,
			"modifier": operator,
		})
	if result.Error != nil {
		return domain.MutationPolicy{}, fmt.Errorf("replace Draft Mutation Policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.MutationPolicy{}, adapter.mutationPolicyWriteConflict(ctx, policy.Code)
	}
	return adapter.GetMutationPolicy(ctx, policy.Code)
}

func (adapter *Adapter) SetMutationPolicyStatus(ctx context.Context, code string, from, to domain.PolicyStatus, operator string) (domain.MutationPolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&mutationPolicyRecord{}).
		Where("code = ? AND status = ?", code, from).
		Updates(map[string]any{"status": to, "modifier": operator})
	if result.Error != nil {
		return domain.MutationPolicy{}, fmt.Errorf("transition Mutation Policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.MutationPolicy{}, adapter.mutationPolicyWriteConflict(ctx, code)
	}
	return adapter.GetMutationPolicy(ctx, code)
}

func (adapter *Adapter) UpdateMutationPolicyMetadata(ctx context.Context, code, name, description, operator string) (domain.MutationPolicy, error) {
	result := adapter.gorm.WithContext(ctx).Model(&mutationPolicyRecord{}).
		Where("code = ? AND status IN ?", code, []domain.PolicyStatus{domain.PolicyStatusActive, domain.PolicyStatusDeprecated}).
		Updates(map[string]any{"name": name, "description": description, "modifier": operator})
	if result.Error != nil {
		return domain.MutationPolicy{}, fmt.Errorf("update Mutation Policy metadata: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.MutationPolicy{}, adapter.mutationPolicyWriteConflict(ctx, code)
	}
	return adapter.GetMutationPolicy(ctx, code)
}

func (adapter *Adapter) DeleteDraftMutationPolicy(ctx context.Context, code string) error {
	result := adapter.gorm.WithContext(ctx).Where("code = ? AND status = ?", code, domain.PolicyStatusDraft).Delete(&mutationPolicyRecord{})
	if result.Error != nil {
		return fmt.Errorf("delete Draft Mutation Policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return adapter.mutationPolicyWriteConflict(ctx, code)
	}
	return nil
}

func (adapter *Adapter) mutationPolicyWriteConflict(ctx context.Context, code string) error {
	if _, err := adapter.GetMutationPolicy(ctx, code); err != nil {
		return err
	}
	return domain.ErrMutationPolicyStateConflict
}

func (record mutationPolicyRecord) policy() domain.MutationPolicy {
	return domain.MutationPolicy{
		Code: record.Code, Name: record.Name, Description: record.Description, TypeCode: record.TypeCode,
		AllowAdd: record.AllowAdd, AllowModify: record.AllowModify, AllowDelete: record.AllowDelete,
		CreateOperatorField: record.CreateOperatorField, CreateTimeField: record.CreateTimeField,
		ModifyOperatorField: record.ModifyOperatorField, ModifyTimeField: record.ModifyTimeField,
		Status: record.Status, Creator: record.Creator, Modifier: record.Modifier,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func (adapter *Adapter) ExecutePageQuery(ctx context.Context, query domain.PageQuery) (result domain.QueryResult, returnErr error) {
	transaction := adapter.gorm.WithContext(ctx).Begin(&sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if transaction.Error != nil {
		return domain.QueryResult{}, fmt.Errorf("%w: begin consistent read", application.ErrQueryUnavailable)
	}
	defer func() {
		if returnErr != nil {
			_ = transaction.Rollback().Error
		}
	}()

	result, returnErr = executePageQuery(ctx, transaction, query)
	if returnErr != nil {
		return domain.QueryResult{}, returnErr
	}
	if err := transaction.Commit().Error; err != nil {
		return domain.QueryResult{}, classifyQueryError(err, ctx.Err(), "commit consistent read")
	}
	return result, nil
}

// ExecuteQuerySnapshot owns the only transaction used by the relational
// Managed Table query path. All Policy reads, live Schema inspection, Count,
// and Scan performed through the callback share this read-only RR session.
func (adapter *Adapter) ExecuteQuerySnapshot(ctx context.Context, execute func(application.QuerySnapshotSession) (domain.QueryResult, error)) (result domain.QueryResult, returnErr error) {
	transaction := adapter.gorm.WithContext(ctx).Begin(&sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if transaction.Error != nil {
		return domain.QueryResult{}, classifyQueryError(transaction.Error, ctx.Err(), "begin Policy Snapshot")
	}

	session := &querySnapshotSession{adapter: adapter, database: transaction}
	session.active.Store(true)
	defer func() {
		if recovered := recover(); recovered != nil {
			session.active.Store(false)
			_ = transaction.Rollback().Error
			panic(recovered)
		}
	}()
	result, returnErr = execute(session)
	session.active.Store(false)
	if returnErr != nil {
		if rollbackErr := transaction.Rollback().Error; rollbackErr != nil {
			return domain.QueryResult{}, classifyQueryError(rollbackErr, ctx.Err(), "rollback Policy Snapshot")
		}
		return domain.QueryResult{}, returnErr
	}
	if err := transaction.Commit().Error; err != nil {
		return domain.QueryResult{}, classifyQueryError(err, ctx.Err(), "commit Policy Snapshot")
	}
	return result, nil
}

type querySnapshotSession struct {
	adapter  *Adapter
	database *gorm.DB
	active   atomic.Bool
}

func (session *querySnapshotSession) available() error {
	if !session.active.Load() {
		return application.ErrQueryUnavailable
	}
	return nil
}

func (session *querySnapshotSession) GetTablePolicy(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	if err := session.available(); err != nil {
		return domain.TablePolicy{}, err
	}
	policy, err := session.adapter.getTablePolicy(ctx, session.database, tableName)
	if err != nil {
		if errors.Is(err, domain.ErrTablePolicyNotFound) {
			return domain.TablePolicy{}, err
		}
		return domain.TablePolicy{}, classifyCatalogSnapshotError(err, ctx.Err(), "read Table Policy")
	}
	return policy, nil
}

func (session *querySnapshotSession) GetQueryPolicy(ctx context.Context, code string) (domain.QueryPolicy, error) {
	if err := session.available(); err != nil {
		return domain.QueryPolicy{}, err
	}
	policy, err := session.adapter.getQueryPolicy(ctx, session.database, code)
	if err != nil {
		return domain.QueryPolicy{}, classifyCatalogSnapshotError(err, ctx.Err(), "read Query Policy")
	}
	return policy, nil
}

func (session *querySnapshotSession) GetMutationPolicy(ctx context.Context, code string) (domain.MutationPolicy, error) {
	if err := session.available(); err != nil {
		return domain.MutationPolicy{}, err
	}
	policy, err := session.adapter.getMutationPolicy(ctx, session.database, code)
	if err != nil {
		return domain.MutationPolicy{}, classifyCatalogSnapshotError(err, ctx.Err(), "read Mutation Policy")
	}
	return policy, nil
}

func (session *querySnapshotSession) GetTableSchema(ctx context.Context, tableName string) (domain.TableSchema, error) {
	if err := session.available(); err != nil {
		return domain.TableSchema{}, err
	}
	schema, err := session.adapter.getTableSchema(ctx, session.database, tableName)
	if err != nil {
		if errors.Is(err, application.ErrDatabaseTableNotFound) || errors.Is(err, application.ErrProtectedTable) {
			return domain.TableSchema{}, err
		}
		return domain.TableSchema{}, classifyQueryError(err, ctx.Err(), "read live Schema")
	}
	return schema, nil
}

func (session *querySnapshotSession) ExecutePageQuery(ctx context.Context, query domain.PageQuery) (domain.QueryResult, error) {
	if err := session.available(); err != nil {
		return domain.QueryResult{}, err
	}
	return executePageQuery(ctx, session.database, query)
}

func classifyCatalogSnapshotError(err, contextErr error, operation string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(contextErr, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %s", application.ErrQueryTimeout, operation)
	}
	return fmt.Errorf("%w: %s", application.ErrPolicyCatalogUnavailable, operation)
}

func executePageQuery(ctx context.Context, database *gorm.DB, query domain.PageQuery) (domain.QueryResult, error) {
	countContext, cancelCount := context.WithTimeout(ctx, 3*time.Second)
	var totalCount int64
	countErr := applyPageQuery(database.WithContext(countContext), query).Count(&totalCount).Error
	countContextErr := countContext.Err()
	cancelCount()
	if countErr != nil {
		return domain.QueryResult{}, classifyQueryError(countErr, countContextErr, "count rows")
	}

	scanContext, cancelScan := context.WithTimeout(ctx, 3*time.Second)
	rows, err := applyPageQuery(database.WithContext(scanContext), query).
		Clauses(selectColumns(query.Columns)).
		Order(clause.OrderByColumn{
			Column: clause.Column{Name: query.Order.Field},
			Desc:   query.Order.Direction == "DESC",
		}).
		Offset(query.Offset).
		Limit(query.PageSize).
		Rows()
	if err != nil {
		scanContextErr := scanContext.Err()
		cancelScan()
		return domain.QueryResult{}, classifyQueryError(err, scanContextErr, "scan rows")
	}
	resultRows, err := scanJSONStringRows(rows, query.Columns)
	scanContextErr := scanContext.Err()
	closeErr := rows.Close()
	cancelScan()
	if err != nil {
		return domain.QueryResult{}, classifyQueryError(err, scanContextErr, "scan rows")
	}
	if closeErr != nil {
		return domain.QueryResult{}, classifyQueryError(closeErr, scanContextErr, "close row stream")
	}

	totalPages := int64(0)
	if totalCount > 0 {
		totalPages = (totalCount + int64(query.PageSize) - 1) / int64(query.PageSize)
	}
	versions, err := queryRecordVersions(ctx, database, query.TableName, resultRows)
	if err != nil {
		return domain.QueryResult{}, classifyQueryError(err, ctx.Err(), "read record versions")
	}
	return domain.QueryResult{
		RecordVersions: versions,
		Columns:        append([]domain.Column(nil), query.Columns...),
		Rows:           resultRows,
		Page: domain.Page{
			PageNumber: query.PageNumber,
			PageSize:   query.PageSize,
			TotalCount: totalCount,
			TotalPages: totalPages,
		},
	}, nil
}

func classifyMutationError(err error, contextErr error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(contextErr, context.DeadlineExceeded) {
		return application.ErrMutationTimeout
	}
	var mysqlError *driver.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1062:
			return application.ErrDuplicateKey
		case 1264, 1265, 1406:
			// Numeric overflow, invalid ENUM members and overlong strings are
			// rejected storage values. Keep these known input errors editable;
			// other database failures remain unavailable.
			return application.ErrInvalidMutation
		}
	}
	return application.ErrMutationUnavailable
}

func applyPageQuery(database *gorm.DB, query domain.PageQuery) *gorm.DB {
	database = database.Session(&gorm.Session{})
	database.Statement.Table = query.TableName
	database = database.Clauses(clause.From{Tables: []clause.Table{{Name: query.TableName}}})
	for _, condition := range query.Conditions {
		column, _ := pageQueryColumn(query.Columns, condition.Field)
		switch condition.Operator {
		case domain.QueryOperatorExact:
			database = database.Where(clause.Eq{
				Column: clause.Column{Name: condition.Field},
				Value:  pageQueryValue(column, *condition.Value),
			})
		case domain.QueryOperatorContains:
			database = database.Where(clause.Expr{
				SQL:  "? LIKE ? ESCAPE '!'",
				Vars: []any{clause.Column{Name: condition.Field}, containsPattern(string(*condition.Value))},
			})
		case domain.QueryOperatorOpenRange, domain.QueryOperatorClosedRange:
			if condition.From != nil {
				value := pageQueryValue(column, *condition.From)
				if condition.Operator == domain.QueryOperatorOpenRange {
					database = database.Where(clause.Gt{Column: clause.Column{Name: condition.Field}, Value: value})
				} else {
					database = database.Where(clause.Gte{Column: clause.Column{Name: condition.Field}, Value: value})
				}
			}
			if condition.To != nil {
				value := pageQueryValue(column, *condition.To)
				if condition.Operator == domain.QueryOperatorOpenRange {
					database = database.Where(clause.Lt{Column: clause.Column{Name: condition.Field}, Value: value})
				} else {
					database = database.Where(clause.Lte{Column: clause.Column{Name: condition.Field}, Value: value})
				}
			}
		case domain.QueryOperatorIn, domain.QueryOperatorNotIn:
			values := make([]any, 0, len(condition.Values))
			for _, item := range condition.Values {
				values = append(values, pageQueryValue(column, *item))
			}
			membership := clause.IN{Column: clause.Column{Name: condition.Field}, Values: values}
			if condition.Operator == domain.QueryOperatorIn {
				database = database.Where(membership)
			} else {
				database = database.Not(membership)
			}
		case domain.QueryOperatorIsNull:
			database = database.Where(clause.Eq{Column: clause.Column{Name: condition.Field}, Value: nil})
		case domain.QueryOperatorIsNotNull:
			database = database.Where(clause.Neq{Column: clause.Column{Name: condition.Field}, Value: nil})
		}
	}
	return database
}

func containsPattern(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	value = strings.ReplaceAll(value, "_", "!_")
	return "%" + value + "%"
}

func selectColumns(columns []domain.Column) clause.Select {
	projections := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		field := clause.Column{Name: column.Name}
		if column.Type == domain.ColumnTypeFloat64 {
			// FLOAT text protocol output has only six significant digits.
			// DOUBLE promotion preserves every stored FLOAT bit before display.
			projections = append(projections, "CAST(? AS DOUBLE) AS ?")
			args = append(args, field, field)
		} else {
			projections = append(projections, "?")
			args = append(args, field)
		}
	}
	return clause.Select{Expression: clause.Expr{SQL: strings.Join(projections, ","), Vars: args}}
}

func pageQueryColumn(columns []domain.Column, name string) (domain.Column, bool) {
	for _, column := range columns {
		if column.Name == name {
			return column, true
		}
	}
	return domain.Column{}, false
}

func pageQueryValue(column domain.Column, value domain.JSONString) any {
	parsed, _ := domain.ParseColumnValue(column, value)
	if column.Type == domain.ColumnTypeJSON {
		return gorm.Expr("CAST(? AS JSON)", parsed)
	}
	return parsed
}

func scanJSONStringRows(rows *sql.Rows, columns []domain.Column) ([]domain.Row, error) {
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]domain.Row, 0)
	for rows.Next() {
		values := make([]any, len(columnNames))
		destinations := make([]any, len(columnNames))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		row := make(domain.Row, len(columnNames))
		for index, name := range columnNames {
			column, found := pageQueryColumn(columns, name)
			if !found {
				return nil, fmt.Errorf("selected column %s is absent from live Schema", name)
			}
			row[name] = jsonStringCell(column, values[index])
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func jsonStringCell(column domain.Column, value any) *domain.JSONString {
	if value == nil {
		return nil
	}
	var text string
	switch typed := value.(type) {
	case []byte:
		text = string(typed)
	case string:
		text = typed
	case int64:
		text = strconv.FormatInt(typed, 10)
	case uint64:
		text = strconv.FormatUint(typed, 10)
	case time.Time:
		text = formatTemporalValue(column.Type, typed)
	default:
		text = fmt.Sprint(typed)
	}
	cell := domain.JSONString(text)
	return &cell
}

func formatTemporalValue(columnType domain.ColumnType, value time.Time) string {
	switch columnType {
	case domain.ColumnTypeDate:
		return value.In(sqlUTC).Format("2006-01-02")
	case domain.ColumnTypeDateTime:
		return formatMicrosecondTime(value.In(sqlUTC), "2006-01-02 15:04:05", "")
	case domain.ColumnTypeTimestamp:
		return formatMicrosecondTime(value.In(sqlUTC), "2006-01-02T15:04:05", "Z")
	default:
		return value.In(sqlUTC).Format(time.RFC3339Nano)
	}
}

func formatMicrosecondTime(value time.Time, layout, suffix string) string {
	formatted := value.Format(layout)
	microseconds := value.Nanosecond() / 1000
	if microseconds != 0 {
		fraction := strings.TrimRight(fmt.Sprintf("%06d", microseconds), "0")
		formatted += "." + fraction
	}
	return formatted + suffix
}

func classifyQueryError(err, contextErr error, operation string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(contextErr, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %s", application.ErrQueryTimeout, operation)
	}
	return fmt.Errorf("%w: %s", application.ErrQueryUnavailable, operation)
}

type metadataRow struct {
	TableName     string         `gorm:"column:table_name"`
	TableComment  string         `gorm:"column:table_comment"`
	PolicyTable   sql.NullString `gorm:"column:policy_table"`
	PolicyEnabled bool           `gorm:"column:policy_enabled"`
	PrimaryColumn sql.NullString `gorm:"column:primary_column"`
}

type tableMetadata struct {
	name          string
	comment       string
	policyExists  bool
	policyEnabled bool
	primaryKey    []string
}

func (adapter *Adapter) readTableMetadata(ctx context.Context) ([]tableMetadata, error) {
	var rows []metadataRow
	err := adapter.gorm.WithContext(ctx).Raw(`
SELECT
  tables.TABLE_NAME AS table_name,
  COALESCE(tables.TABLE_COMMENT, '') AS table_comment,
  policies.table_name AS policy_table,
  COALESCE(policies.enabled, 0) AS policy_enabled,
  primary_keys.COLUMN_NAME AS primary_column
FROM information_schema.TABLES AS tables
LEFT JOIN rcc_table_policies AS policies
  ON policies.table_name = tables.TABLE_NAME
LEFT JOIN information_schema.KEY_COLUMN_USAGE AS primary_keys
  ON primary_keys.TABLE_SCHEMA = tables.TABLE_SCHEMA
  AND primary_keys.TABLE_NAME = tables.TABLE_NAME
  AND primary_keys.CONSTRAINT_NAME = 'PRIMARY'
WHERE tables.TABLE_SCHEMA = ?
  AND tables.TABLE_TYPE = 'BASE TABLE'
ORDER BY tables.TABLE_NAME, primary_keys.ORDINAL_POSITION`, adapter.database).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("discover database tables: %w", err)
	}

	byName := make(map[string]*tableMetadata, len(rows))
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		if protectedTable(row.TableName) {
			continue
		}
		item, found := byName[row.TableName]
		if !found {
			item = &tableMetadata{
				name:          row.TableName,
				comment:       row.TableComment,
				policyExists:  row.PolicyTable.Valid,
				policyEnabled: row.PolicyTable.Valid && row.PolicyEnabled,
			}
			byName[row.TableName] = item
			order = append(order, row.TableName)
		}
		if row.PrimaryColumn.Valid {
			item.primaryKey = append(item.primaryKey, row.PrimaryColumn.String)
		}
	}

	metadata := make([]tableMetadata, 0, len(order))
	for _, name := range order {
		metadata = append(metadata, *byName[name])
	}
	return metadata, nil
}

func protectedTable(tableName string) bool {
	return strings.HasPrefix(strings.ToLower(tableName), "rcc_")
}

var sqlUTC = time.UTC

var _ application.TableMetadataReader = (*Adapter)(nil)
var _ application.Readiness = (*Adapter)(nil)
var _ application.QueryExecutor = (*Adapter)(nil)
var _ application.QuerySnapshotExecutor = (*Adapter)(nil)
var _ domain.TablePolicyCatalog = (*Adapter)(nil)
var _ domain.QueryPolicyCatalog = (*Adapter)(nil)
var _ domain.MutationPolicyCatalog = (*Adapter)(nil)

func liveTextCapacity(dataType string, capacity sql.NullInt64) uint64 {
	switch strings.ToLower(dataType) {
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext":
		if capacity.Valid && capacity.Int64 > 0 {
			return uint64(capacity.Int64)
		}
	}
	return 0
}
