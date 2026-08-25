package application

import (
	"context"
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
	ErrInvalidPolicySnapshot    = errors.New("invalid Policy Snapshot")
)

// QueryExecutor is the deep execution interface implemented by the MySQL
// adapter. GORM sessions, compilation, transactions, and rollback stay behind
// this seam.
type QueryExecutor interface {
	ExecutePageQuery(context.Context, domain.PageQuery) (domain.QueryResult, error)
}

// QuerySnapshotSession is the request-scoped view of the Managed Data Source
// exposed by infrastructure. Its Catalog methods are deliberately separate:
// a Policy Snapshot is three aggregate reads, never a JOIN or a cache lookup.
// Implementations must reject use after ExecuteQuerySnapshot returns.
type QuerySnapshotSession interface {
	QueryExecutor
	GetTablePolicy(context.Context, string) (domain.TablePolicy, error)
	GetQueryPolicy(context.Context, string) (domain.QueryPolicy, error)
	GetMutationPolicy(context.Context, string) (domain.MutationPolicy, error)
	GetTableSchema(context.Context, string) (domain.TableSchema, error)
}

// QuerySnapshotExecutor owns the read-only REPEATABLE READ transaction used
// for one Managed Table query. Transaction/session details never cross this
// application port; the callback only receives the bounded domain operations
// that must share the snapshot.
type QuerySnapshotExecutor interface {
	ExecuteQuerySnapshot(context.Context, func(QuerySnapshotSession) (domain.QueryResult, error)) (domain.QueryResult, error)
}

// ManagedTableQuery is the single-method query module used by the HTTP
// interface. It owns Policy Snapshot loading, live Schema validation, and
// fresh strategy construction for every request.
type ManagedTableQuery struct {
	snapshotExecutor QuerySnapshotExecutor
	queryTypes       *QueryPolicyTypeRegistry
	mutationTypes    *MutationPolicyTypeRegistry
}

func NewManagedTableQuery(executor QuerySnapshotExecutor, queryTypes *QueryPolicyTypeRegistry, mutationTypes *MutationPolicyTypeRegistry) *ManagedTableQuery {
	return &ManagedTableQuery{snapshotExecutor: executor, queryTypes: queryTypes, mutationTypes: mutationTypes}
}

func (query *ManagedTableQuery) Execute(ctx context.Context, tableName string, spec domain.QuerySpec) (domain.QueryResult, error) {
	if protectedTable(tableName) {
		return domain.QueryResult{}, ErrProtectedTable
	}
	return query.executePolicySnapshot(ctx, tableName, spec)
}

func (query *ManagedTableQuery) executePolicySnapshot(ctx context.Context, tableName string, spec domain.QuerySpec) (domain.QueryResult, error) {
	return query.snapshotExecutor.ExecuteQuerySnapshot(ctx, func(session QuerySnapshotSession) (domain.QueryResult, error) {
		tablePolicy, err := session.GetTablePolicy(ctx, tableName)
		if err != nil {
			if errors.Is(err, domain.ErrTablePolicyNotFound) {
				return domain.QueryResult{}, err
			}
			return domain.QueryResult{}, snapshotCatalogError(err, "Table Policy")
		}

		// Keep the aggregate reads visibly separate. In particular, do not
		// short-circuit a disabled assignment into a partial Policy Snapshot.
		queryPolicy, err := session.GetQueryPolicy(ctx, tablePolicy.QueryPolicyCode)
		if err != nil {
			return domain.QueryResult{}, snapshotCatalogError(err, "Query Policy")
		}
		mutationPolicy, err := session.GetMutationPolicy(ctx, tablePolicy.MutationPolicyCode)
		if err != nil {
			return domain.QueryResult{}, snapshotCatalogError(err, "Mutation Policy")
		}
		if !tablePolicy.Enabled {
			return domain.QueryResult{}, ErrTablePolicyDisabled
		}
		if !runtimePolicyStatus(queryPolicy.Status) || !runtimePolicyStatus(mutationPolicy.Status) {
			return domain.QueryResult{}, fmt.Errorf("%w: referenced Policy is not executable", ErrInvalidPolicySnapshot)
		}

		strategy, err := query.queryTypes.Build(queryPolicy)
		if err != nil {
			return domain.QueryResult{}, err
		}
		if err := query.mutationTypes.Validate(mutationPolicy); err != nil {
			return domain.QueryResult{}, err
		}

		schema, err := session.GetTableSchema(ctx, tableName)
		if err != nil {
			if errors.Is(err, ErrDatabaseTableNotFound) || errors.Is(err, ErrProtectedTable) {
				return domain.QueryResult{}, err
			}
			if errors.Is(err, ErrQueryTimeout) {
				return domain.QueryResult{}, err
			}
			return domain.QueryResult{}, fmt.Errorf("%w: read live Schema", ErrQueryUnavailable)
		}
		if !schema.Compatible {
			return domain.QueryResult{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
		}
		if err := query.queryTypes.ValidateForTable(queryPolicy, schema); err != nil {
			return domain.QueryResult{}, err
		}
		if err := query.mutationTypes.ValidateForTable(mutationPolicy, schema); err != nil {
			return domain.QueryResult{}, err
		}

		return strategy.execute(ctx, schema, spec, session)
	})
}

func snapshotCatalogError(err error, aggregate string) error {
	if errors.Is(err, ErrQueryTimeout) {
		return err
	}
	return fmt.Errorf("%w: read %s", ErrPolicyCatalogUnavailable, aggregate)
}

func runtimePolicyStatus(status domain.PolicyStatus) bool {
	return status == domain.PolicyStatusActive || status == domain.PolicyStatusDeprecated
}
