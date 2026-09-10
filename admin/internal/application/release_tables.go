package application

import (
	"context"
	"encoding/hex"
	"errors"
	"slices"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func sortedTableKeys[T any](tables map[string]T) []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func releaseTableNames(items []ReleaseItem) []string {
	tables := map[string]bool{}
	for _, item := range items {
		tables[item.TableName] = true
	}
	return sortedTableKeys(tables)
}

// RemapReleaseItemError maps a table batch failure into the whole-order coordinates.
func RemapReleaseItemError(err error, positions []int) error {
	var itemError *ReleaseItemError
	if errors.As(err, &itemError) && itemError.Index >= 0 && itemError.Index < len(positions) {
		return &ReleaseItemError{Index: positions[itemError.Index], Cause: itemError.Cause}
	}
	return &ReleaseItemError{Index: positions[0], Cause: err}
}

func guardDraftReleaseTable(ctx context.Context, session ReleaseOrderSession, table string) error {
	_, err := session.GetTablePolicy(ctx, table)
	if err != nil && !errors.Is(err, domain.ErrTablePolicyNotFound) {
		return policySnapshotCatalogError(err, "Table Policy", mutationPolicySnapshot)
	}
	return err
}

// ReverseReleaseItemError changes original-publication coordinates to restoration coordinates.
func ReverseReleaseItemError(err error, count int) error {
	var itemError *ReleaseItemError
	if errors.As(err, &itemError) && itemError.Index >= 0 && itemError.Index < count {
		return &ReleaseItemError{Index: count - 1 - itemError.Index, Cause: itemError.Cause}
	}
	return err
}

// Guards must all be held before any consistent read, regardless of the DML
// order. PublicationTable holds metadata only and cannot reorder business intent.
func (r *ReleaseOrders) resolveReleaseTables(ctx context.Context, s ReleaseOrderSession, items []ReleaseItem, publishing bool) (map[string]PublicationTable, error) {
	positions := map[string][]int{}
	for index, item := range items {
		positions[item.TableName] = append(positions[item.TableName], index)
	}
	names := sortedTableKeys(positions)
	for _, name := range names {
		var err error
		if publishing {
			err = s.(PublicationSession).LockPublicationTable(ctx, name)
		} else {
			err = guardDraftReleaseTable(ctx, s, name)
		}
		if err != nil {
			return nil, RemapReleaseItemError(err, positions[name])
		}
	}
	tables := map[string]PublicationTable{}
	for _, name := range names {
		execution, err := s.LockAndReadTableExecutionSchema(ctx, name)
		if err != nil {
			return nil, RemapReleaseItemError(err, positions[name])
		}
		snapshot, err := r.snapshots.resolve(ctx, s, name, mutationPolicySnapshot)
		if err != nil {
			return nil, RemapReleaseItemError(err, positions[name])
		}
		tables[name] = PublicationTable{Schema: snapshot.schema, Execution: execution, Policy: snapshot.mutationPolicy, SchemaDigest: hex.EncodeToString(releaseDigest(execution))}
	}
	return tables, nil
}

func frozenReleaseTables(tables map[string]PublicationTable) map[string]domain.ReleaseExecutionSnapshot {
	frozen := map[string]domain.ReleaseExecutionSnapshot{}
	for name, table := range tables {
		frozen[name] = domain.ReleaseExecutionSnapshot{Schema: table.Execution, Mutation: domain.NewReleaseMutationSemantics(table.Policy)}
	}
	return frozen
}

func frozenOrderDigest(order ReleaseOrder) string {
	// Successful results never become part of the independently frozen application.
	items := append([]ReleaseItem(nil), order.Items...)
	for index := range items {
		items[index].Publication = nil
		items[index].Rollback = nil
	}
	return hex.EncodeToString(releaseDigest(struct {
		Title  string
		Items  []ReleaseItem
		Tables map[string]domain.ReleaseExecutionSnapshot
	}{order.Title, items, order.FrozenTables}))
}

func verifyFrozenTables(order ReleaseOrder, tables map[string]PublicationTable) error {
	if len(order.FrozenTables) != len(tables) || frozenOrderDigest(order) != order.FrozenDigest {
		return ErrReleaseFrozenChanged
	}
	for index, item := range order.Items {
		table := tables[item.TableName]
		current := domain.ReleaseExecutionSnapshot{Schema: table.Execution, Mutation: domain.NewReleaseMutationSemantics(table.Policy)}
		if hex.EncodeToString(releaseDigest(current)) != hex.EncodeToString(releaseDigest(order.FrozenTables[item.TableName])) {
			return &ReleaseItemError{Index: index, Cause: ErrReleaseFrozenChanged}
		}
	}
	return nil
}
