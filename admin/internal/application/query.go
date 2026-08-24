package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrTablePolicyDisabled      = errors.New("Table Policy is disabled")
	ErrInvalidQueryCondition    = errors.New("invalid query condition")
	ErrInvalidQueryOrder        = errors.New("invalid query order")
	ErrInvalidPagination        = errors.New("invalid pagination")
	ErrPolicyCatalogUnavailable = errors.New("Policy Catalog unavailable")
	ErrQueryUnavailable         = errors.New("query unavailable")
	ErrQueryTimeout             = errors.New("query timeout")
)

// QueryExecutor is the deep execution interface implemented by the MySQL
// adapter. GORM sessions, compilation, transactions, and rollback stay behind
// this seam.
type QueryExecutor interface {
	ExecutePageQuery(context.Context, domain.PageQuery) (domain.QueryResult, error)
}

// ManagedTableQuery is the single-method query module used by the HTTP
// interface. It owns Policy Snapshot loading, live Schema validation, and
// fresh strategy construction for every request.
type ManagedTableQuery struct {
	metadata TableMetadataReader
	catalog  domain.TablePolicyCatalog
	registry *StrategyRegistry
	executor QueryExecutor
}

func NewManagedTableQuery(metadata TableMetadataReader, catalog domain.TablePolicyCatalog, registry *StrategyRegistry, executor QueryExecutor) *ManagedTableQuery {
	return &ManagedTableQuery{metadata: metadata, catalog: catalog, registry: registry, executor: executor}
}

func (query *ManagedTableQuery) Execute(ctx context.Context, tableName string, spec domain.QuerySpec) (domain.QueryResult, error) {
	if protectedTable(tableName) {
		return domain.QueryResult{}, ErrProtectedTable
	}

	policy, err := query.catalog.Get(ctx, tableName)
	if err != nil {
		if !errors.Is(err, domain.ErrTablePolicyNotFound) {
			return domain.QueryResult{}, fmt.Errorf("%w", ErrPolicyCatalogUnavailable)
		}
		return domain.QueryResult{}, err
	}
	if !policy.Enabled {
		return domain.QueryResult{}, ErrTablePolicyDisabled
	}

	schema, err := query.metadata.GetTableSchema(ctx, tableName)
	if err != nil {
		if !errors.Is(err, ErrDatabaseTableNotFound) && !errors.Is(err, ErrProtectedTable) {
			return domain.QueryResult{}, fmt.Errorf("%w", ErrQueryUnavailable)
		}
		return domain.QueryResult{}, err
	}
	if !schema.Compatible {
		return domain.QueryResult{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}

	strategy, err := query.registry.Query(policy.QueryPolicy, json.RawMessage(policy.QueryPolicyConfig))
	if err != nil {
		return domain.QueryResult{}, err
	}
	mutation, err := query.registry.Mutation(policy.MutationPolicy, json.RawMessage(policy.MutationPolicyConfig))
	if err != nil {
		return domain.QueryResult{}, err
	}
	if err := strategy.validateSchema(schema); err != nil {
		return domain.QueryResult{}, err
	}
	if err := mutation.validateSchema(schema); err != nil {
		return domain.QueryResult{}, err
	}

	return strategy.execute(ctx, schema, spec, query.executor)
}
