package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrProtectedTable          = errors.New("protected table")
	ErrIncompatibleTable       = errors.New("incompatible table")
	ErrInvalidPolicyDefinition = errors.New("invalid Policy definition")
)

type CreateTablePolicy struct {
	TableName            string
	QueryPolicy          string
	QueryPolicyConfig    json.RawMessage
	MutationPolicy       string
	MutationPolicyConfig json.RawMessage
	AllowAdd             bool
	AllowModify          bool
	AllowDelete          bool
}

// TablePolicyManagement coordinates Catalog access, live table validation,
// and strategy construction without exposing storage details to HTTP callers.
type TablePolicyManagement struct {
	metadata TableMetadataReader
	catalog  domain.TablePolicyCatalog
	registry *StrategyRegistry
	operator string
}

func NewTablePolicyManagement(metadata TableMetadataReader, catalog domain.TablePolicyCatalog, registry *StrategyRegistry, operator string) *TablePolicyManagement {
	return &TablePolicyManagement{metadata: metadata, catalog: catalog, registry: registry, operator: operator}
}

func (management *TablePolicyManagement) Create(ctx context.Context, candidate CreateTablePolicy) (domain.TablePolicy, error) {
	if strings.TrimSpace(candidate.TableName) == "" ||
		strings.TrimSpace(candidate.QueryPolicy) == "" || len(candidate.QueryPolicyConfig) == 0 ||
		strings.TrimSpace(candidate.MutationPolicy) == "" || len(candidate.MutationPolicyConfig) == 0 {
		return domain.TablePolicy{}, ErrInvalidPolicyDefinition
	}
	if protectedTable(candidate.TableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	table, err := management.metadata.GetDatabaseTable(ctx, candidate.TableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if !table.Compatible {
		return domain.TablePolicy{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *table.IncompatibilityReason)
	}
	if err := management.registry.Validate(candidate.QueryPolicy, candidate.QueryPolicyConfig, candidate.MutationPolicy, candidate.MutationPolicyConfig); err != nil {
		return domain.TablePolicy{}, err
	}

	policy := domain.TablePolicy{
		TableName:            candidate.TableName,
		QueryPolicy:          candidate.QueryPolicy,
		QueryPolicyConfig:    append(domain.JSONConfig(nil), candidate.QueryPolicyConfig...),
		MutationPolicy:       candidate.MutationPolicy,
		MutationPolicyConfig: append(domain.JSONConfig(nil), candidate.MutationPolicyConfig...),
		AllowAdd:             candidate.AllowAdd,
		AllowModify:          candidate.AllowModify,
		AllowDelete:          candidate.AllowDelete,
		Enabled:              false,
	}
	if err := management.catalog.Create(ctx, policy, management.operator); err != nil {
		return domain.TablePolicy{}, err
	}
	return policy, nil
}

func (management *TablePolicyManagement) List(ctx context.Context) ([]domain.TablePolicy, error) {
	return management.catalog.List(ctx)
}

func (management *TablePolicyManagement) Get(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	return management.catalog.Get(ctx, tableName)
}

func (management *TablePolicyManagement) Replace(ctx context.Context, tableName string, candidate CreateTablePolicy) (domain.TablePolicy, error) {
	if candidate.TableName != tableName {
		return domain.TablePolicy{}, ErrInvalidPolicyDefinition
	}
	if strings.TrimSpace(candidate.TableName) == "" ||
		strings.TrimSpace(candidate.QueryPolicy) == "" || len(candidate.QueryPolicyConfig) == 0 ||
		strings.TrimSpace(candidate.MutationPolicy) == "" || len(candidate.MutationPolicyConfig) == 0 {
		return domain.TablePolicy{}, ErrInvalidPolicyDefinition
	}
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	schema, err := management.metadata.GetTableSchema(ctx, tableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if !schema.Compatible {
		return domain.TablePolicy{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}
	if err := management.registry.ValidateForSchema(candidate.QueryPolicy, candidate.QueryPolicyConfig, candidate.MutationPolicy, candidate.MutationPolicyConfig, schema); err != nil {
		return domain.TablePolicy{}, err
	}
	return management.catalog.Replace(ctx, domain.TablePolicy{
		TableName:            tableName,
		QueryPolicy:          candidate.QueryPolicy,
		QueryPolicyConfig:    append(domain.JSONConfig(nil), candidate.QueryPolicyConfig...),
		MutationPolicy:       candidate.MutationPolicy,
		MutationPolicyConfig: append(domain.JSONConfig(nil), candidate.MutationPolicyConfig...),
		AllowAdd:             candidate.AllowAdd,
		AllowModify:          candidate.AllowModify,
		AllowDelete:          candidate.AllowDelete,
	}, management.operator)
}

func (management *TablePolicyManagement) Enable(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	policy, err := management.catalog.Get(ctx, tableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	schema, err := management.metadata.GetTableSchema(ctx, tableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if !schema.Compatible {
		return domain.TablePolicy{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}
	if err := management.registry.ValidateForSchema(
		policy.QueryPolicy,
		json.RawMessage(policy.QueryPolicyConfig),
		policy.MutationPolicy,
		json.RawMessage(policy.MutationPolicyConfig),
		schema,
	); err != nil {
		return domain.TablePolicy{}, err
	}
	return management.catalog.SetEnabled(ctx, tableName, true, management.operator)
}

func (management *TablePolicyManagement) Disable(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	return management.catalog.SetEnabled(ctx, tableName, false, management.operator)
}

func protectedTable(tableName string) bool {
	return strings.HasPrefix(strings.ToLower(tableName), "rcc_")
}
