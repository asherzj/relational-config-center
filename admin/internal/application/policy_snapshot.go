package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// PolicySnapshotReader exposes the four independent reads required to resolve
// one complete Policy Snapshot. Query and Mutation sessions both satisfy this
// interface while retaining their different transaction and execution rules.
type PolicySnapshotReader interface {
	GetTablePolicy(context.Context, string) (domain.TablePolicy, error)
	GetQueryPolicy(context.Context, string) (domain.QueryPolicy, error)
	GetMutationPolicy(context.Context, string) (domain.MutationPolicy, error)
	GetTableSchema(context.Context, string) (domain.TableSchema, error)
}

type policySnapshotMode uint8

const (
	queryPolicySnapshot policySnapshotMode = iota
	mutationPolicySnapshot
)

type resolvedPolicySnapshot struct {
	tablePolicy    domain.TablePolicy
	queryPolicy    domain.QueryPolicy
	mutationPolicy domain.MutationPolicy
	schema         domain.TableSchema
	queryStrategy  QueryStrategy
}

// policySnapshotResolver owns the complete fail-closed resolution shared by
// Managed Table Query and Mutation. Transactions remain owned by their MySQL
// session adapters; the resolver only uses the bounded reads above.
type policySnapshotResolver struct {
	queryTypes    *QueryPolicyTypeRegistry
	mutationTypes *MutationPolicyTypeRegistry
}

func newPolicySnapshotResolver(queryTypes *QueryPolicyTypeRegistry, mutationTypes *MutationPolicyTypeRegistry) *policySnapshotResolver {
	return &policySnapshotResolver{queryTypes: queryTypes, mutationTypes: mutationTypes}
}

func (resolver *policySnapshotResolver) resolve(ctx context.Context, reader PolicySnapshotReader, tableName string, mode policySnapshotMode) (resolvedPolicySnapshot, error) {
	tablePolicy, err := reader.GetTablePolicy(ctx, tableName)
	if err != nil {
		if errors.Is(err, domain.ErrTablePolicyNotFound) {
			return resolvedPolicySnapshot{}, err
		}
		return resolvedPolicySnapshot{}, policySnapshotCatalogError(err, "Table Policy", mode)
	}

	// Keep the aggregate reads visibly separate. A disabled assignment is still
	// resolved completely, so callers never observe a partial Policy Snapshot.
	queryPolicy, err := reader.GetQueryPolicy(ctx, tablePolicy.QueryPolicyCode)
	if err != nil {
		return resolvedPolicySnapshot{}, policySnapshotCatalogError(err, "Query Policy", mode)
	}
	mutationPolicy, err := reader.GetMutationPolicy(ctx, tablePolicy.MutationPolicyCode)
	if err != nil {
		return resolvedPolicySnapshot{}, policySnapshotCatalogError(err, "Mutation Policy", mode)
	}
	if !tablePolicy.Enabled {
		return resolvedPolicySnapshot{}, ErrTablePolicyDisabled
	}
	if !runtimePolicyStatus(queryPolicy.Status) || !runtimePolicyStatus(mutationPolicy.Status) {
		return resolvedPolicySnapshot{}, fmt.Errorf("%w: referenced Policy is not executable", ErrInvalidPolicySnapshot)
	}

	queryStrategy, err := resolver.queryTypes.Build(queryPolicy)
	if err != nil {
		return resolvedPolicySnapshot{}, err
	}
	if err := resolver.mutationTypes.Validate(mutationPolicy); err != nil {
		return resolvedPolicySnapshot{}, err
	}

	schema, err := reader.GetTableSchema(ctx, tableName)
	if err != nil {
		return resolvedPolicySnapshot{}, policySnapshotSchemaError(err, mode)
	}
	if !schema.Compatible {
		return resolvedPolicySnapshot{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}
	if err := resolver.queryTypes.ValidateForTable(queryPolicy, schema); err != nil {
		return resolvedPolicySnapshot{}, err
	}
	if err := resolver.mutationTypes.ValidateForTable(mutationPolicy, schema); err != nil {
		return resolvedPolicySnapshot{}, err
	}

	return resolvedPolicySnapshot{
		tablePolicy: tablePolicy, queryPolicy: queryPolicy, mutationPolicy: mutationPolicy,
		schema: schema, queryStrategy: queryStrategy,
	}, nil
}

func policySnapshotCatalogError(err error, aggregate string, mode policySnapshotMode) error {
	if mode == queryPolicySnapshot && errors.Is(err, ErrQueryTimeout) {
		return err
	}
	if mode == mutationPolicySnapshot && (errors.Is(err, ErrMutationTimeout) || errors.Is(err, context.DeadlineExceeded)) {
		return fmt.Errorf("%w: read %s", ErrMutationTimeout, aggregate)
	}
	return fmt.Errorf("%w: read %s", ErrPolicyCatalogUnavailable, aggregate)
}

func policySnapshotSchemaError(err error, mode policySnapshotMode) error {
	if errors.Is(err, ErrDatabaseTableNotFound) || errors.Is(err, ErrProtectedTable) {
		return err
	}
	if mode == queryPolicySnapshot {
		if errors.Is(err, ErrQueryTimeout) {
			return err
		}
		return fmt.Errorf("%w: read live Schema", ErrQueryUnavailable)
	}
	if errors.Is(err, ErrMutationTimeout) {
		return err
	}
	return fmt.Errorf("%w: read live Schema", ErrMutationUnavailable)
}

func runtimePolicyStatus(status domain.PolicyStatus) bool {
	return status == domain.PolicyStatusActive || status == domain.PolicyStatusDeprecated
}
