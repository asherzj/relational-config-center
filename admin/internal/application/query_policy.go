package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

const (
	PageQueryPolicyType = "page_query"
	maxPolicyPageSize   = 200
)

var (
	ErrInvalidQueryPolicyDefinition = errors.New("invalid Query Policy definition")
	ErrInvalidPolicyCode            = errors.New("invalid Policy Code")
	ErrUnknownQueryPolicyType       = errors.New("unknown Query Policy Type")
	ErrInvalidQueryPolicyRules      = errors.New("invalid Query Policy rules")
	ErrInvalidPolicyTransition      = errors.New("invalid Policy lifecycle transition")
	ErrQueryPolicyNotAssignable     = errors.New("Query Policy is not assignable")

	policyCodePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*_v[1-9][0-9]*$`)
	fieldNamePattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	technologyPrefixes = []string{
		"mysql_", "mariadb_", "postgres_", "postgresql_", "sqlite_",
		"oracle_", "sqlserver_", "mongodb_", "gorm_", "sql_",
	}
)

type QueryPolicyType struct {
	Code string
}

// QueryPolicyTypeRegistry is code-owned and immutable after construction. It
// intentionally exposes a finite list rather than reflection or plugins.
type QueryPolicyTypeRegistry struct {
	builders map[string]func(domain.QueryPolicy) QueryStrategy
}

func NewQueryPolicyTypeRegistry() *QueryPolicyTypeRegistry {
	return &QueryPolicyTypeRegistry{builders: map[string]func(domain.QueryPolicy) QueryStrategy{
		PageQueryPolicyType: func(policy domain.QueryPolicy) QueryStrategy {
			return &pageQueryStrategy{config: pageQueryConfig{
				DefaultOrder:    &defaultOrder{Field: policy.DefaultOrderField, Direction: policy.DefaultOrderDirection},
				DefaultPageSize: policy.DefaultPageSize,
				MaxPageSize:     policy.MaxPageSize,
			}}
		},
	}}
}

func (registry *QueryPolicyTypeRegistry) List() []QueryPolicyType {
	return []QueryPolicyType{{Code: PageQueryPolicyType}}
}

func (registry *QueryPolicyTypeRegistry) Validate(policy domain.QueryPolicy) error {
	if _, found := registry.builders[policy.TypeCode]; !found {
		return fmt.Errorf("%w: %s", ErrUnknownQueryPolicyType, policy.TypeCode)
	}
	return registry.validatePersistentScalars(policy)
}

// Build selects a fresh executor by relational Type Code and passes the
// concrete Policy's scalar values directly. Policy JSON and legacy strategy
// identifiers are intentionally absent from this path.
func (registry *QueryPolicyTypeRegistry) Build(policy domain.QueryPolicy) (QueryStrategy, error) {
	builder, found := registry.builders[policy.TypeCode]
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrUnknownQueryPolicyType, policy.TypeCode)
	}
	if err := registry.Validate(policy); err != nil {
		return nil, err
	}
	return builder(policy), nil
}

func (registry *QueryPolicyTypeRegistry) ValidateForTable(policy domain.QueryPolicy, schema domain.TableSchema) error {
	if err := registry.Validate(policy); err != nil {
		return err
	}
	if _, found := schema.Column(policy.DefaultOrderField); !found {
		return fmt.Errorf("%w: default order field does not exist", ErrInvalidQueryPolicyRules)
	}
	for _, column := range schema.Columns {
		if column.Type == domain.ColumnTypeUnsupported {
			return fmt.Errorf("%w: unsupported live column %s", ErrIncompatibleTable, column.Name)
		}
	}
	return nil
}

func (*QueryPolicyTypeRegistry) validatePersistentScalars(policy domain.QueryPolicy) error {
	if !fieldNamePattern.MatchString(policy.DefaultOrderField) {
		return fmt.Errorf("%w: unsafe default order field", ErrInvalidQueryPolicyRules)
	}
	if policy.DefaultOrderDirection != "ASC" && policy.DefaultOrderDirection != "DESC" {
		return fmt.Errorf("%w: order direction must be ASC or DESC", ErrInvalidQueryPolicyRules)
	}
	if policy.DefaultPageSize < 1 || policy.MaxPageSize < 1 || policy.DefaultPageSize > policy.MaxPageSize {
		return fmt.Errorf("%w: invalid page size range", ErrInvalidQueryPolicyRules)
	}
	if policy.DefaultPageSize > maxPolicyPageSize || policy.MaxPageSize > maxPolicyPageSize {
		return fmt.Errorf("%w: page size exceeds platform safety limit", ErrInvalidQueryPolicyRules)
	}
	return nil
}

type PutQueryPolicy struct {
	Code                  string
	Name                  string
	Description           string
	TypeCode              string
	DefaultOrderField     string
	DefaultOrderDirection string
	DefaultPageSize       int
	MaxPageSize           int
}

type QueryPolicyManagement struct {
	catalog  domain.QueryPolicyCatalog
	registry *QueryPolicyTypeRegistry
}

func NewQueryPolicyManagement(catalog domain.QueryPolicyCatalog, registry *QueryPolicyTypeRegistry) *QueryPolicyManagement {
	return &QueryPolicyManagement{catalog: catalog, registry: registry}
}

func (management *QueryPolicyManagement) Types() []QueryPolicyType {
	return management.registry.List()
}

func (management *QueryPolicyManagement) Create(ctx context.Context, candidate PutQueryPolicy) (domain.QueryPolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.QueryPolicy{}, identityErr
	}
	policy, err := draftQueryPolicy(candidate)
	if err != nil {
		return domain.QueryPolicy{}, err
	}
	if err := management.registry.validatePersistentScalars(policy); err != nil {
		return domain.QueryPolicy{}, err
	}
	return management.catalog.CreateQueryPolicy(ctx, policy, operator)
}

func (management *QueryPolicyManagement) List(ctx context.Context) ([]domain.QueryPolicy, error) {
	return management.catalog.ListQueryPolicies(ctx)
}

func (management *QueryPolicyManagement) Get(ctx context.Context, code string) (domain.QueryPolicy, error) {
	return management.catalog.GetQueryPolicy(ctx, code)
}

// GetForNewAssignment rejects Draft and Deprecated definitions. Existing
// assignments intentionally use a different runtime path so Deprecated remains
// executable without becoming newly assignable.
func (management *QueryPolicyManagement) GetForNewAssignment(ctx context.Context, code string) (domain.QueryPolicy, error) {
	policy, err := management.catalog.GetQueryPolicy(ctx, code)
	if err != nil {
		return domain.QueryPolicy{}, err
	}
	if policy.Status != domain.PolicyStatusActive {
		return domain.QueryPolicy{}, ErrQueryPolicyNotAssignable
	}
	return policy, nil
}

// ValidateForTable verifies both the code-owned Type contract and the parts of
// the concrete definition that depend on the live Managed Table Schema.
func (management *QueryPolicyManagement) ValidateForTable(policy domain.QueryPolicy, schema domain.TableSchema) error {
	return management.registry.ValidateForTable(policy, schema)
}

func (management *QueryPolicyManagement) ReplaceDraft(ctx context.Context, code string, candidate PutQueryPolicy) (domain.QueryPolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.QueryPolicy{}, identityErr
	}
	if candidate.Code != code {
		return domain.QueryPolicy{}, ErrInvalidPolicyCode
	}
	policy, err := draftQueryPolicy(candidate)
	if err != nil {
		return domain.QueryPolicy{}, err
	}
	if err := management.registry.validatePersistentScalars(policy); err != nil {
		return domain.QueryPolicy{}, err
	}
	return management.catalog.ReplaceDraftQueryPolicy(ctx, policy, operator)
}

func (management *QueryPolicyManagement) Activate(ctx context.Context, code string) (domain.QueryPolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.QueryPolicy{}, identityErr
	}
	policy, err := management.catalog.GetQueryPolicy(ctx, code)
	if err != nil {
		return domain.QueryPolicy{}, err
	}
	if policy.Status != domain.PolicyStatusDraft {
		return domain.QueryPolicy{}, ErrInvalidPolicyTransition
	}
	if err := management.registry.Validate(policy); err != nil {
		return domain.QueryPolicy{}, err
	}
	return management.catalog.SetQueryPolicyStatus(ctx, code, domain.PolicyStatusDraft, domain.PolicyStatusActive, operator)
}

func (management *QueryPolicyManagement) Deprecate(ctx context.Context, code string) (domain.QueryPolicy, error) {
	return management.transition(ctx, code, domain.PolicyStatusActive, domain.PolicyStatusDeprecated)
}

func (management *QueryPolicyManagement) transition(ctx context.Context, code string, from, to domain.PolicyStatus) (domain.QueryPolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.QueryPolicy{}, identityErr
	}
	policy, err := management.catalog.GetQueryPolicy(ctx, code)
	if err != nil {
		return domain.QueryPolicy{}, err
	}
	if policy.Status != from {
		return domain.QueryPolicy{}, ErrInvalidPolicyTransition
	}
	return management.catalog.SetQueryPolicyStatus(ctx, code, from, to, operator)
}

func (management *QueryPolicyManagement) UpdateMetadata(ctx context.Context, code, name, description string) (domain.QueryPolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.QueryPolicy{}, identityErr
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 500 {
		return domain.QueryPolicy{}, ErrInvalidQueryPolicyDefinition
	}
	policy, err := management.catalog.GetQueryPolicy(ctx, code)
	if err != nil {
		return domain.QueryPolicy{}, err
	}
	if policy.Status != domain.PolicyStatusActive && policy.Status != domain.PolicyStatusDeprecated {
		return domain.QueryPolicy{}, ErrInvalidPolicyTransition
	}
	return management.catalog.UpdateQueryPolicyMetadata(ctx, code, name, description, operator)
}

func (management *QueryPolicyManagement) DeleteDraft(ctx context.Context, code string) error {
	if _, identityErr := requestOperator(ctx); identityErr != nil {
		return identityErr
	}
	policy, err := management.catalog.GetQueryPolicy(ctx, code)
	if err != nil {
		return err
	}
	if policy.Status != domain.PolicyStatusDraft {
		return ErrInvalidPolicyTransition
	}
	return management.catalog.DeleteDraftQueryPolicy(ctx, code)
}

func draftQueryPolicy(candidate PutQueryPolicy) (domain.QueryPolicy, error) {
	code := strings.TrimSpace(candidate.Code)
	if !validPolicyCode(code) {
		return domain.QueryPolicy{}, ErrInvalidPolicyCode
	}
	name := strings.TrimSpace(candidate.Name)
	description := strings.TrimSpace(candidate.Description)
	typeCode := strings.TrimSpace(candidate.TypeCode)
	orderField := strings.TrimSpace(candidate.DefaultOrderField)
	direction := strings.TrimSpace(candidate.DefaultOrderDirection)
	if name == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 500 || typeCode == "" || len(typeCode) > 64 ||
		orderField == "" || len(orderField) > 64 || direction == "" || len(direction) > 8 ||
		candidate.DefaultPageSize < -2147483648 || candidate.DefaultPageSize > 2147483647 ||
		candidate.MaxPageSize < -2147483648 || candidate.MaxPageSize > 2147483647 {
		return domain.QueryPolicy{}, ErrInvalidQueryPolicyDefinition
	}
	return domain.QueryPolicy{
		Code:                  code,
		Name:                  name,
		Description:           description,
		TypeCode:              typeCode,
		DefaultOrderField:     orderField,
		DefaultOrderDirection: direction,
		DefaultPageSize:       candidate.DefaultPageSize,
		MaxPageSize:           candidate.MaxPageSize,
		Status:                domain.PolicyStatusDraft,
	}, nil
}

func validPolicyCode(code string) bool {
	if len(code) > 100 || !policyCodePattern.MatchString(code) {
		return false
	}
	for _, prefix := range technologyPrefixes {
		if strings.HasPrefix(code, prefix) {
			return false
		}
	}
	return true
}
