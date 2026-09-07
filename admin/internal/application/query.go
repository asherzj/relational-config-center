package application

import (
	"context"
	"errors"

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
	PolicySnapshotReader
	QueryExecutor
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
	snapshots        *policySnapshotResolver
}

func NewManagedTableQuery(executor QuerySnapshotExecutor, queryTypes *QueryPolicyTypeRegistry, mutationTypes *MutationPolicyTypeRegistry) *ManagedTableQuery {
	return &ManagedTableQuery{snapshotExecutor: executor, snapshots: newPolicySnapshotResolver(queryTypes, mutationTypes)}
}

func (query *ManagedTableQuery) Execute(ctx context.Context, tableName string, spec domain.QuerySpec) (domain.QueryResult, error) {
	if protectedTable(tableName) {
		return domain.QueryResult{}, ErrProtectedTable
	}
	return query.executePolicySnapshot(ctx, tableName, spec)
}

func (query *ManagedTableQuery) executePolicySnapshot(ctx context.Context, tableName string, spec domain.QuerySpec) (domain.QueryResult, error) {
	return query.snapshotExecutor.ExecuteQuerySnapshot(ctx, func(session QuerySnapshotSession) (domain.QueryResult, error) {
		snapshot, err := query.snapshots.resolve(ctx, session, tableName, queryPolicySnapshot)
		if err != nil {
			return domain.QueryResult{}, err
		}
		return snapshot.queryStrategy.execute(ctx, snapshot.schema, spec, session)
	})
}
