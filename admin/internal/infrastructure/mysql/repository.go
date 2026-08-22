package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository executes policy-approved managed-table operations.
type Repository struct {
	db       *gorm.DB
	compiler queryCompiler
}

// NewRepository creates a MySQL managed-table repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Query returns one stable page and the full filtered row count.
func (r *Repository) Query(ctx context.Context, policy domain.Policy, query domain.QuerySpec) (domain.PageResult, error) {
	countQuery, err := r.compiler.applyFilter(r.db.WithContext(ctx).Table(policy.Table), policy, query.Filter)
	if err != nil {
		return domain.PageResult{}, err
	}
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return domain.PageResult{}, classify("count rows", err)
	}

	dataQuery, err := r.compiler.applyFilter(r.db.WithContext(ctx).Table(policy.Table), policy, query.Filter)
	if err != nil {
		return domain.PageResult{}, err
	}
	dataQuery = dataQuery.Clauses(readableSelect(policy), orderBy(policy, query.Sort))
	offset := (query.Page.Number - 1) * query.Page.Size
	rows := make([]map[string]any, 0, query.Page.Size)
	if err := dataQuery.Offset(offset).Limit(query.Page.Size).Find(&rows).Error; err != nil {
		return domain.PageResult{}, classify("query rows", err)
	}
	for _, row := range rows {
		normalizeRow(policy, row)
	}
	return domain.PageResult{
		Rows: rows,
		Page: domain.PageInfo{
			Number: query.Page.Number,
			Size:   query.Page.Size,
			Total:  total,
		},
	}, nil
}

// Create inserts one row and returns an auto-increment key when configured.
func (r *Repository) Create(ctx context.Context, policy domain.Policy, values map[string]any) (domain.MutationResult, error) {
	physical := physicalValues(policy, values)
	result := domain.MutationResult{}
	if key, exists := values[policy.PrimaryKey]; exists {
		result.Key = externalKey(key)
	}
	primary := policy.Fields[policy.PrimaryKey]
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created := tx.Table(policy.Table).Create(physical)
		if created.Error != nil {
			return created.Error
		}
		result.AffectedRows = created.RowsAffected
		if primary.AutoIncrement && result.Key == "" {
			var key uint64
			if err := tx.Raw("SELECT LAST_INSERT_ID()").Scan(&key).Error; err != nil {
				return err
			}
			result.Key = strconv.FormatUint(key, 10)
		}
		return nil
	})
	if err != nil {
		return domain.MutationResult{}, classify("create row", err)
	}
	return result, nil
}

// Update changes one primary-key-selected row.
func (r *Repository) Update(ctx context.Context, policy domain.Policy, key any, values map[string]any) (domain.MutationResult, error) {
	primary := policy.Fields[policy.PrimaryKey]
	updated := r.db.WithContext(ctx).
		Table(policy.Table).
		Where(clause.Eq{Column: clause.Column{Name: primary.Column}, Value: key}).
		Updates(physicalValues(policy, values))
	if updated.Error != nil {
		return domain.MutationResult{}, classify("update row", updated.Error)
	}
	if updated.RowsAffected == 0 {
		return domain.MutationResult{}, domain.ErrRowNotFound
	}
	return domain.MutationResult{AffectedRows: updated.RowsAffected, Key: externalKey(key)}, nil
}

// Delete removes one primary-key-selected row.
func (r *Repository) Delete(ctx context.Context, policy domain.Policy, key any) (domain.MutationResult, error) {
	primary := policy.Fields[policy.PrimaryKey]
	deleted := r.db.WithContext(ctx).
		Table(policy.Table).
		Where(clause.Eq{Column: clause.Column{Name: primary.Column}, Value: key}).
		Delete(&map[string]any{})
	if deleted.Error != nil {
		return domain.MutationResult{}, classify("delete row", deleted.Error)
	}
	if deleted.RowsAffected == 0 {
		return domain.MutationResult{}, domain.ErrRowNotFound
	}
	return domain.MutationResult{AffectedRows: deleted.RowsAffected, Key: externalKey(key)}, nil
}

func readableSelect(policy domain.Policy) clause.Select {
	names := make([]string, 0, len(policy.Fields))
	for name, field := range policy.Fields {
		if field.Readable {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	columns := make([]clause.Column, 0, len(names))
	for _, name := range names {
		field := policy.Fields[name]
		columns = append(columns, clause.Column{
			Table: policy.Table,
			Name:  field.Column,
			Alias: name,
		})
	}
	return clause.Select{Columns: columns}
}

func orderBy(policy domain.Policy, sorts []domain.Sort) clause.OrderBy {
	columns := make([]clause.OrderByColumn, 0, len(sorts))
	for _, item := range sorts {
		field := policy.Fields[item.Field]
		columns = append(columns, clause.OrderByColumn{
			Column: clause.Column{Table: policy.Table, Name: field.Column},
			Desc:   item.Direction == domain.DirectionDescending,
		})
	}
	return clause.OrderBy{Columns: columns}
}

func physicalValues(policy domain.Policy, values map[string]any) map[string]any {
	physical := make(map[string]any, len(values))
	for publicName, value := range values {
		physical[policy.Fields[publicName].Column] = value
	}
	return physical
}

func normalizeRow(policy domain.Policy, row map[string]any) {
	for publicName, field := range policy.Fields {
		if !field.Readable {
			continue
		}
		value, exists := row[publicName]
		if !exists || value == nil {
			continue
		}
		row[publicName] = normalizeDatabaseValue(field.Type, value)
	}
}

func normalizeDatabaseValue(valueType domain.ValueType, value any) any {
	rawBytes, isBytes := value.([]byte)
	switch valueType {
	case domain.TypeJSON:
		encoded := rawBytes
		if text, ok := value.(string); ok {
			encoded = []byte(text)
			isBytes = true
		}
		if !isBytes {
			return value
		}
		var decoded any
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err == nil {
			return decoded
		}
	case domain.TypeString:
		if isBytes {
			return string(rawBytes)
		}
	case domain.TypeBoolean:
		if isBytes {
			parsed, err := strconv.ParseBool(string(rawBytes))
			if err == nil {
				return parsed
			}
		}
		switch number := value.(type) {
		case int64:
			return number != 0
		case uint64:
			return number != 0
		}
	case domain.TypeInteger:
		if isBytes {
			return string(rawBytes)
		}
		switch number := value.(type) {
		case int64:
			return strconv.FormatInt(number, 10)
		case int:
			return strconv.Itoa(number)
		}
	case domain.TypeUnsigned:
		if isBytes {
			return string(rawBytes)
		}
		switch number := value.(type) {
		case uint64:
			return strconv.FormatUint(number, 10)
		case uint:
			return strconv.FormatUint(uint64(number), 10)
		}
	case domain.TypeTime:
		if isBytes {
			if parsed, ok := parseMySQLTime(string(rawBytes)); ok {
				return parsed
			}
		}
	}
	return value
}

func externalKey(key any) string {
	switch value := key.(type) {
	case string:
		return value
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case uint:
		return strconv.FormatUint(uint64(value), 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	default:
		return fmt.Sprint(value)
	}
}

func parseMySQLTime(value string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", time.RFC3339Nano} {
		parsed, err := time.ParseInLocation(layout, value, time.UTC)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func classify(action string, err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrForeignKeyViolated) {
		return fmt.Errorf("%s: %w", action, domain.ErrConflict)
	}
	var mysqlError *drivermysql.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1062, 1451, 1452:
			return fmt.Errorf("%s: %w", action, domain.ErrConflict)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
