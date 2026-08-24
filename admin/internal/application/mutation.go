package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrMutationNotAllowed   = errors.New("mutation operation is not allowed")
	ErrInvalidMutation      = errors.New("invalid mutation content")
	ErrMissingRequiredField = errors.New("required mutation field is missing")
	ErrDuplicateKey         = errors.New("duplicate key")
	ErrMutationRowNotFound  = errors.New("mutation row not found")
	ErrMutationUnavailable  = errors.New("mutation unavailable")
	ErrMutationTimeout      = errors.New("mutation timeout")
)

// MutationExecutor is the single deep execution interface exposed by the
// Mutation module. GORM, dynamic clauses, transactions, affected-row rules,
// duplicate-key inspection, and LastInsertId remain inside its MySQL adapter.
type MutationExecutor interface {
	InsertRow(context.Context, domain.RowInsert) (string, error)
	UpdateRow(context.Context, domain.RowUpdate) (int64, error)
	DeleteRow(context.Context, domain.RowDelete) (int64, error)
}

// OperatorProvider supplies the deployment-attributed Operator used by
// server-owned Auto Fill rules. V1 wires a fixed implementation, while the
// interface leaves the attribution source replaceable.
type OperatorProvider interface {
	Operator(context.Context) (domain.JSONString, error)
}

type fixedOperatorProvider struct {
	value domain.JSONString
}

func NewFixedOperatorProvider(value string) OperatorProvider {
	return fixedOperatorProvider{value: domain.JSONString(value)}
}

func (provider fixedOperatorProvider) Operator(context.Context) (domain.JSONString, error) {
	return provider.value, nil
}

// ManagedTableMutation owns Policy Snapshot loading, live Schema validation,
// and fresh Mutation Policy construction for every request.
type ManagedTableMutation struct {
	metadata TableMetadataReader
	catalog  domain.TablePolicyCatalog
	registry *StrategyRegistry
	operator OperatorProvider
	executor MutationExecutor
}

func NewManagedTableMutation(metadata TableMetadataReader, catalog domain.TablePolicyCatalog, registry *StrategyRegistry, operator OperatorProvider, executor MutationExecutor) *ManagedTableMutation {
	return &ManagedTableMutation{metadata: metadata, catalog: catalog, registry: registry, operator: operator, executor: executor}
}

func (mutation *ManagedTableMutation) Add(ctx context.Context, tableName string, content domain.MutationContent) (string, error) {
	schema, strategy, err := mutation.currentStrategy(ctx, tableName)
	if err != nil {
		return "", err
	}
	return strategy.add(ctx, schema, content, mutation.operator, mutation.executor)
}

func (mutation *ManagedTableMutation) Modify(ctx context.Context, tableName string, id domain.JSONString, content domain.MutationContent) (int64, error) {
	schema, strategy, err := mutation.currentStrategy(ctx, tableName)
	if err != nil {
		return 0, err
	}
	return strategy.modify(ctx, schema, id, content, mutation.operator, mutation.executor)
}

func (mutation *ManagedTableMutation) Delete(ctx context.Context, tableName string, id domain.JSONString) (int64, error) {
	schema, strategy, err := mutation.currentDeleteStrategy(ctx, tableName)
	if err != nil {
		return 0, err
	}
	return strategy.delete(ctx, schema, id, mutation.executor)
}

// currentDeleteStrategy deliberately avoids full query and mutation Schema
// validation. DELETE depends only on the current Managed Table invariant and
// its id type, so unrelated unsupported columns and Auto Fill fields cannot
// disable an otherwise safe hard delete.
func (mutation *ManagedTableMutation) currentDeleteStrategy(ctx context.Context, tableName string) (domain.TableSchema, MutationStrategy, error) {
	schema, policy, err := mutation.currentSnapshot(ctx, tableName)
	if err != nil {
		return domain.TableSchema{}, nil, err
	}

	// Construct both configured policies to reject unknown or malformed current
	// snapshots, but do not apply full-row Schema validation for DELETE.
	if _, err := mutation.registry.Query(policy.QueryPolicy, json.RawMessage(policy.QueryPolicyConfig)); err != nil {
		return domain.TableSchema{}, nil, err
	}
	strategy, err := mutation.registry.Mutation(policy.MutationPolicy, json.RawMessage(policy.MutationPolicyConfig))
	if err != nil {
		return domain.TableSchema{}, nil, err
	}
	return schema, strategy, nil
}

func (mutation *ManagedTableMutation) currentStrategy(ctx context.Context, tableName string) (domain.TableSchema, MutationStrategy, error) {
	schema, policy, err := mutation.currentSnapshot(ctx, tableName)
	if err != nil {
		return domain.TableSchema{}, nil, err
	}

	if _, err := mutation.registry.Query(policy.QueryPolicy, json.RawMessage(policy.QueryPolicyConfig)); err != nil {
		return domain.TableSchema{}, nil, err
	}
	strategy, err := mutation.registry.Mutation(policy.MutationPolicy, json.RawMessage(policy.MutationPolicyConfig))
	if err != nil {
		return domain.TableSchema{}, nil, err
	}
	if err := strategy.validateSchema(schema); err != nil {
		return domain.TableSchema{}, nil, err
	}
	return schema, strategy, nil
}

func (mutation *ManagedTableMutation) currentSnapshot(ctx context.Context, tableName string) (domain.TableSchema, domain.TablePolicy, error) {
	if protectedTable(tableName) {
		return domain.TableSchema{}, domain.TablePolicy{}, ErrProtectedTable
	}

	policy, err := mutation.catalog.Get(ctx, tableName)
	if err != nil {
		if !errors.Is(err, domain.ErrTablePolicyNotFound) {
			return domain.TableSchema{}, domain.TablePolicy{}, ErrPolicyCatalogUnavailable
		}
		return domain.TableSchema{}, domain.TablePolicy{}, err
	}
	if !policy.Enabled {
		return domain.TableSchema{}, domain.TablePolicy{}, ErrTablePolicyDisabled
	}

	schema, err := mutation.metadata.GetTableSchema(ctx, tableName)
	if err != nil {
		if !errors.Is(err, ErrDatabaseTableNotFound) && !errors.Is(err, ErrProtectedTable) {
			return domain.TableSchema{}, domain.TablePolicy{}, ErrMutationUnavailable
		}
		return domain.TableSchema{}, domain.TablePolicy{}, err
	}
	if !schema.Compatible {
		return domain.TableSchema{}, domain.TablePolicy{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}
	return schema, policy, nil
}
