package mysql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// LegacyPolicyDiagnostic identifies one Catalog row that cannot be represented
// by the relational Policy definitions without changing its behavior.
type LegacyPolicyDiagnostic struct {
	TableName string
	Problem   string
}

// LegacyPolicyPreflightError is returned before any definition or Table Policy
// row is rewritten. Diagnostics are stable and sorted for operator action.
type LegacyPolicyPreflightError struct {
	Diagnostics []LegacyPolicyDiagnostic
}

func (err *LegacyPolicyPreflightError) Error() string {
	parts := make([]string, 0, len(err.Diagnostics))
	for _, diagnostic := range err.Diagnostics {
		parts = append(parts, diagnostic.TableName+": "+diagnostic.Problem)
	}
	return "legacy Policy migration preflight failed: " + strings.Join(parts, "; ")
}

type legacyQueryConfig struct {
	DefaultOrder    *legacyDefaultOrder `json:"default_order,omitempty"`
	DefaultPageSize int                 `json:"default_page_size,omitempty"`
	MaxPageSize     int                 `json:"max_page_size,omitempty"`
}

type legacyDefaultOrder struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type legacyMutationConfig struct {
	AutoFill *legacyAutoFill `json:"auto_fill,omitempty"`
}

type legacyAutoFill struct {
	Add    map[string]legacyAutoFillRule `json:"add,omitempty"`
	Modify map[string]legacyAutoFillRule `json:"modify,omitempty"`
}

type legacyAutoFillRule struct {
	Source string  `json:"source"`
	Value  *string `json:"value,omitempty"`
}

type legacyBackfillPlan struct {
	record   legacyPolicyRecord
	query    domain.QueryPolicy
	mutation domain.MutationPolicy
}

const (
	legacyPageQueryStrategy           = "mysql_page_query_v1"
	legacySingleTableMutationStrategy = "mysql_single_table_mutation_v1"
)

// legacyPolicyRecord is deliberately confined to the upgrade utility. The
// running Admin adapter never reads or writes these contracted columns.
type legacyPolicyRecord struct {
	ID                   uint64         `gorm:"column:id;primaryKey"`
	Table                string         `gorm:"column:table_name"`
	QueryPolicyCode      sql.NullString `gorm:"column:query_policy_code"`
	MutationPolicyCode   sql.NullString `gorm:"column:mutation_policy_code"`
	QueryPolicy          string         `gorm:"column:query_policy"`
	QueryPolicyConfig    string         `gorm:"column:query_policy_config"`
	MutationPolicy       string         `gorm:"column:mutation_policy"`
	MutationPolicyConfig string         `gorm:"column:mutation_policy_config"`
	AllowAdd             bool           `gorm:"column:allow_add"`
	AllowModify          bool           `gorm:"column:allow_modify"`
	AllowDelete          bool           `gorm:"column:allow_delete"`
}

func (legacyPolicyRecord) TableName() string { return "rcc_table_policies" }

// MigrateLegacyTablePolicies runs a set-wide preflight and then performs one
// all-or-nothing Catalog backfill. Expand migration 005 and audit migration 013 must be applied first.
func (adapter *Adapter) MigrateLegacyTablePolicies(ctx context.Context, operator string) error {
	return adapter.gorm.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var records []legacyPolicyRecord
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Order("table_name ASC").Find(&records).Error; err != nil {
			return fmt.Errorf("lock legacy Table Policies: %w", err)
		}

		plans := make([]legacyBackfillPlan, 0, len(records))
		diagnostics := make([]LegacyPolicyDiagnostic, 0)
		for _, record := range records {
			if record.QueryPolicyCode.Valid || record.MutationPolicyCode.Valid {
				if !record.QueryPolicyCode.Valid || !record.MutationPolicyCode.Valid {
					diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "partial Policy Code backfill; both references must be empty or present"})
				}
				continue
			}
			plan, problems := planLegacyBackfill(record)
			if len(problems) > 0 {
				for _, problem := range problems {
					diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: problem})
				}
				continue
			}
			plans = append(plans, plan)
		}
		if len(diagnostics) > 0 {
			sort.Slice(diagnostics, func(i, j int) bool {
				if diagnostics[i].TableName == diagnostics[j].TableName {
					return diagnostics[i].Problem < diagnostics[j].Problem
				}
				return diagnostics[i].TableName < diagnostics[j].TableName
			})
			return &LegacyPolicyPreflightError{Diagnostics: diagnostics}
		}

		for _, plan := range plans {
			if err := ensureBackfilledQueryPolicy(transaction, plan.query, operator); err != nil {
				return err
			}
			if err := ensureBackfilledMutationPolicy(transaction, plan.mutation, operator); err != nil {
				return err
			}
			result := transaction.Model(&legacyPolicyRecord{}).Where("id = ? AND query_policy_code IS NULL AND mutation_policy_code IS NULL", plan.record.ID).
				UpdateColumns(map[string]any{"query_policy_code": plan.query.Code, "mutation_policy_code": plan.mutation.Code})
			if result.Error != nil {
				return fmt.Errorf("backfill Table Policy %s: %w", plan.record.Table, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("backfill Table Policy %s: Catalog row changed during migration", plan.record.Table)
			}
		}
		return nil
	})
}

// PreflightLegacyPolicyContraction is the destructive-DDL gate. It validates
// the complete set under row locks and returns sorted, table-scoped
// diagnostics. It deliberately performs no writes and no DDL.
func (adapter *Adapter) PreflightLegacyPolicyContraction(ctx context.Context) error {
	return adapter.gorm.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var records []legacyPolicyRecord
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Order("table_name ASC").Find(&records).Error; err != nil {
			return fmt.Errorf("lock Table Policies for contraction: %w", err)
		}

		queryTypes := application.NewQueryPolicyTypeRegistry()
		mutationTypes := application.NewMutationPolicyTypeRegistry()
		diagnostics := make([]LegacyPolicyDiagnostic, 0)
		for _, record := range records {
			_, legacyProblems := planLegacyBackfill(record)
			for _, problem := range legacyProblems {
				diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "unsupported legacy configuration: " + problem})
			}
			if !record.QueryPolicyCode.Valid || strings.TrimSpace(record.QueryPolicyCode.String) == "" {
				diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "query_policy_code is missing"})
			}
			if !record.MutationPolicyCode.Valid || strings.TrimSpace(record.MutationPolicyCode.String) == "" {
				diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "mutation_policy_code is missing"})
			}
			if !record.QueryPolicyCode.Valid || !record.MutationPolicyCode.Valid {
				continue
			}
			if !validContractionPolicyCode(record.QueryPolicyCode.String) {
				diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "query_policy_code is not a valid technology-neutral versioned Code"})
			}
			if !validContractionPolicyCode(record.MutationPolicyCode.String) {
				diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "mutation_policy_code is not a valid technology-neutral versioned Code"})
			}

			var query queryPolicyRecord
			if err := transaction.Where("code = ?", record.QueryPolicyCode.String).First(&query).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "Query Policy definition " + record.QueryPolicyCode.String + " does not exist"})
			} else if err != nil {
				return fmt.Errorf("read Query Policy %s for contraction: %w", record.QueryPolicyCode.String, err)
			} else {
				policy := query.policy()
				if policy.Status != domain.PolicyStatusActive && policy.Status != domain.PolicyStatusDeprecated {
					diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "Query Policy " + policy.Code + " is not executable (status " + string(policy.Status) + ")"})
				}
				if err := queryTypes.Validate(policy); err != nil {
					diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "Query Policy " + policy.Code + " is not executable: " + err.Error()})
				}
			}

			var mutation mutationPolicyRecord
			if err := transaction.Where("code = ?", record.MutationPolicyCode.String).First(&mutation).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "Mutation Policy definition " + record.MutationPolicyCode.String + " does not exist"})
			} else if err != nil {
				return fmt.Errorf("read Mutation Policy %s for contraction: %w", record.MutationPolicyCode.String, err)
			} else {
				policy := mutation.policy()
				if policy.Status != domain.PolicyStatusActive && policy.Status != domain.PolicyStatusDeprecated {
					diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "Mutation Policy " + policy.Code + " is not executable (status " + string(policy.Status) + ")"})
				}
				if err := mutationTypes.Validate(policy); err != nil {
					diagnostics = append(diagnostics, LegacyPolicyDiagnostic{TableName: record.Table, Problem: "Mutation Policy " + policy.Code + " is not executable: " + err.Error()})
				}
			}
		}
		if len(diagnostics) == 0 {
			return nil
		}
		sort.Slice(diagnostics, func(i, j int) bool {
			if diagnostics[i].TableName == diagnostics[j].TableName {
				return diagnostics[i].Problem < diagnostics[j].Problem
			}
			return diagnostics[i].TableName < diagnostics[j].TableName
		})
		return &LegacyPolicyPreflightError{Diagnostics: diagnostics}
	})
}

// ContractLegacyTablePolicies runs the rich gate first, then performs the same
// single final ALTER as migration 006. The standalone SQL migration contains
// its own SQL-level gate for DBA execution without this command.
func (adapter *Adapter) ContractLegacyTablePolicies(ctx context.Context) error {
	if err := adapter.PreflightLegacyPolicyContraction(ctx); err != nil {
		return err
	}
	if err := adapter.gorm.WithContext(ctx).Exec(`ALTER TABLE rcc_table_policies
  MODIFY COLUMN query_policy_code varchar(100) NOT NULL,
  MODIFY COLUMN mutation_policy_code varchar(100) NOT NULL,
  DROP COLUMN query_policy,
  DROP COLUMN query_policy_config,
  DROP COLUMN mutation_policy,
  DROP COLUMN mutation_policy_config,
  DROP COLUMN allow_add,
  DROP COLUMN allow_modify,
  DROP COLUMN allow_delete,
  ADD CONSTRAINT chk_table_policy_enabled CHECK (enabled IN (0, 1))`).Error; err != nil {
		return fmt.Errorf("contract legacy Table Policy columns: %w", err)
	}
	return nil
}

var contractionPolicyCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*_v[1-9][0-9]*$`)

func validContractionPolicyCode(code string) bool {
	if len(code) > 100 || !contractionPolicyCodePattern.MatchString(code) {
		return false
	}
	for _, prefix := range []string{
		"mysql_", "mariadb_", "postgres_", "postgresql_", "sqlite_",
		"oracle_", "sqlserver_", "mongodb_", "gorm_", "sql_",
	} {
		if strings.HasPrefix(code, prefix) {
			return false
		}
	}
	return true
}

func planLegacyBackfill(record legacyPolicyRecord) (legacyBackfillPlan, []string) {
	problems := make([]string, 0)
	if record.QueryPolicy != legacyPageQueryStrategy {
		problems = append(problems, "unsupported legacy Query strategy "+record.QueryPolicy)
	}
	if record.MutationPolicy != legacySingleTableMutationStrategy {
		problems = append(problems, "unsupported legacy Mutation strategy "+record.MutationPolicy)
	}

	query, err := normalizedLegacyQueryPolicy(record.QueryPolicyConfig)
	if err != nil {
		problems = append(problems, err.Error())
	}
	mutation, err := normalizedLegacyMutationPolicy(record.MutationPolicyConfig, record.AllowAdd, record.AllowModify, record.AllowDelete)
	if err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return legacyBackfillPlan{}, problems
	}
	return legacyBackfillPlan{record: record, query: query, mutation: mutation}, nil
}

func normalizedLegacyQueryPolicy(raw string) (domain.QueryPolicy, error) {
	var config legacyQueryConfig
	if err := decodeLegacyObject([]byte(raw), &config); err != nil {
		return domain.QueryPolicy{}, fmt.Errorf("query_policy_config is not canonical: %v", err)
	}
	field, direction := "id", "DESC"
	if config.DefaultOrder != nil {
		field, direction = strings.TrimSpace(config.DefaultOrder.Field), config.DefaultOrder.Direction
	}
	defaultPageSize, maxPageSize := config.DefaultPageSize, config.MaxPageSize
	if defaultPageSize == 0 {
		defaultPageSize = 20
	}
	if maxPageSize == 0 {
		maxPageSize = 200
	}
	policy := domain.QueryPolicy{
		TypeCode: application.PageQueryPolicyType, DefaultOrderField: field, DefaultOrderDirection: direction,
		DefaultPageSize: defaultPageSize, MaxPageSize: maxPageSize, Status: domain.PolicyStatusActive,
	}
	if err := application.NewQueryPolicyTypeRegistry().Validate(policy); err != nil {
		return domain.QueryPolicy{}, fmt.Errorf("query_policy_config is not representable: %v", err)
	}
	canonical := fmt.Sprintf("%s\x00%s\x00%d\x00%d", field, direction, defaultPageSize, maxPageSize)
	policy.Code = deterministicLegacyCode("legacy_page_query", canonical)
	policy.Name = "Migrated legacy page query"
	policy.Description = "Deterministically backfilled from canonical inline query defaults"
	return policy, nil
}

func normalizedLegacyMutationPolicy(raw string, allowAdd, allowModify, allowDelete bool) (domain.MutationPolicy, error) {
	var config legacyMutationConfig
	if err := decodeLegacyObject([]byte(raw), &config); err != nil {
		return domain.MutationPolicy{}, fmt.Errorf("mutation_policy_config is not canonical: %v", err)
	}
	policy := domain.MutationPolicy{
		TypeCode: application.SingleTableMutationPolicyType,
		AllowAdd: allowAdd, AllowModify: allowModify, AllowDelete: allowDelete,
		Status: domain.PolicyStatusActive,
	}
	if config.AutoFill != nil {
		for field, modifyRule := range config.AutoFill.Modify {
			addRule, mirrored := config.AutoFill.Add[field]
			if !mirrored || addRule.Source != modifyRule.Source || !sameLiteral(addRule.Value, modifyRule.Value) {
				return domain.MutationPolicy{}, fmt.Errorf("Auto Fill field %s in MODIFY must be mirrored identically in ADD", field)
			}
		}
		addOnly := make(map[string]legacyAutoFillRule)
		for field, rule := range config.AutoFill.Add {
			if _, modifies := config.AutoFill.Modify[field]; !modifies {
				addOnly[field] = rule
			}
		}
		var err error
		policy.CreateOperatorField, policy.CreateTimeField, err = fixedLegacySlots(addOnly, "Create")
		if err != nil {
			return domain.MutationPolicy{}, err
		}
		policy.ModifyOperatorField, policy.ModifyTimeField, err = fixedLegacySlots(config.AutoFill.Modify, "Modify")
		if err != nil {
			return domain.MutationPolicy{}, err
		}
	}
	if err := application.NewMutationPolicyTypeRegistry().Validate(policy); err != nil {
		return domain.MutationPolicy{}, fmt.Errorf("Mutation permissions or Auto Fill are not representable: %v", err)
	}
	canonicalParts := []string{
		fmt.Sprint(allowAdd), fmt.Sprint(allowModify), fmt.Sprint(allowDelete),
		optionalCanonical(policy.CreateOperatorField), optionalCanonical(policy.CreateTimeField),
		optionalCanonical(policy.ModifyOperatorField), optionalCanonical(policy.ModifyTimeField),
	}
	policy.Code = deterministicLegacyCode("legacy_mutation", strings.Join(canonicalParts, "\x00"))
	policy.Name = "Migrated legacy mutation"
	policy.Description = "Deterministically backfilled from Mutation permissions and standard audit Auto Fill"
	return policy, nil
}

func fixedLegacySlots(rules map[string]legacyAutoFillRule, label string) (*string, *string, error) {
	var operatorField, timeField *string
	fields := make([]string, 0, len(rules))
	for field := range rules {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		rule := rules[field]
		if rule.Value != nil || rule.Source == "literal" {
			return nil, nil, fmt.Errorf("%s Auto Fill field %s uses unsupported literal data", label, field)
		}
		value := field
		switch rule.Source {
		case "operator":
			if operatorField != nil {
				return nil, nil, fmt.Errorf("%s Auto Fill has multiple operator targets (%s, %s)", label, *operatorField, field)
			}
			operatorField = &value
		case "now":
			if timeField != nil {
				return nil, nil, fmt.Errorf("%s Auto Fill has multiple time targets (%s, %s)", label, *timeField, field)
			}
			timeField = &value
		default:
			return nil, nil, fmt.Errorf("%s Auto Fill field %s uses unsupported source %q", label, field, rule.Source)
		}
	}
	return operatorField, timeField, nil
}

func ensureBackfilledQueryPolicy(transaction *gorm.DB, policy domain.QueryPolicy, operator string) error {
	var existing queryPolicyRecord
	err := transaction.Where("code = ?", policy.Code).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		record := queryPolicyRecord{
			Code: policy.Code, Name: policy.Name, Description: policy.Description, TypeCode: policy.TypeCode,
			DefaultOrderField: policy.DefaultOrderField, DefaultOrderDirection: policy.DefaultOrderDirection,
			DefaultPageSize: policy.DefaultPageSize, MaxPageSize: policy.MaxPageSize, Status: domain.PolicyStatusActive,
			Creator: operator, Modifier: operator,
		}
		if err := transaction.Create(&record).Error; err != nil {
			return fmt.Errorf("create backfilled Query Policy %s: %w", policy.Code, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read backfilled Query Policy %s: %w", policy.Code, err)
	}
	if existing.Status != domain.PolicyStatusActive || existing.TypeCode != policy.TypeCode || existing.DefaultOrderField != policy.DefaultOrderField || existing.DefaultOrderDirection != policy.DefaultOrderDirection || existing.DefaultPageSize != policy.DefaultPageSize || existing.MaxPageSize != policy.MaxPageSize {
		return fmt.Errorf("deterministic Query Policy Code %s already has different semantics", policy.Code)
	}
	return nil
}

func ensureBackfilledMutationPolicy(transaction *gorm.DB, policy domain.MutationPolicy, operator string) error {
	var existing mutationPolicyRecord
	err := transaction.Where("code = ?", policy.Code).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		record := mutationPolicyRecord{
			Code: policy.Code, Name: policy.Name, Description: policy.Description, TypeCode: policy.TypeCode,
			AllowAdd: policy.AllowAdd, AllowModify: policy.AllowModify, AllowDelete: policy.AllowDelete,
			CreateOperatorField: policy.CreateOperatorField, CreateTimeField: policy.CreateTimeField,
			ModifyOperatorField: policy.ModifyOperatorField, ModifyTimeField: policy.ModifyTimeField,
			Status: domain.PolicyStatusActive, Creator: operator, Modifier: operator,
		}
		if err := transaction.Create(&record).Error; err != nil {
			return fmt.Errorf("create backfilled Mutation Policy %s: %w", policy.Code, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read backfilled Mutation Policy %s: %w", policy.Code, err)
	}
	if existing.Status != domain.PolicyStatusActive || existing.TypeCode != policy.TypeCode || existing.AllowAdd != policy.AllowAdd || existing.AllowModify != policy.AllowModify || existing.AllowDelete != policy.AllowDelete || !sameOptional(existing.CreateOperatorField, policy.CreateOperatorField) || !sameOptional(existing.CreateTimeField, policy.CreateTimeField) || !sameOptional(existing.ModifyOperatorField, policy.ModifyOperatorField) || !sameOptional(existing.ModifyTimeField, policy.ModifyTimeField) {
		return fmt.Errorf("deterministic Mutation Policy Code %s already has different semantics", policy.Code)
	}
	return nil
}

func decodeLegacyObject(raw []byte, destination any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' {
		return errors.New("must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("contains trailing JSON")
	}
	return nil
}

func deterministicLegacyCode(prefix, canonical string) string {
	digest := sha256.Sum256([]byte(canonical))
	return prefix + "_" + hex.EncodeToString(digest[:6]) + "_v1"
}

func sameLiteral(left, right *string) bool { return sameOptional(left, right) }

func sameOptional(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func optionalCanonical(value *string) string {
	if value == nil {
		return "-"
	}
	return *value
}
