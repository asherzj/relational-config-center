package application

import (
	"context"
	"errors"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrConcurrencyKeyInvalid = errors.New("invalid concurrency control key")
	ErrConcurrencyKeyInUse   = errors.New("unfinished release details reference this table")
	ErrConcurrencyKeyValue   = errors.New("concurrency key value cannot be determined at draft save")
)

type ConcurrencyKeyField struct {
	Name     string            `json:"name"`
	Type     domain.ColumnType `json:"type"`
	Eligible bool              `json:"eligible"`
}

// Candidate fields come from live metadata and the selected Mutation Policy,
// including assignments that are being created or are currently disabled.
func (m *TablePolicyManagement) ConcurrencyKeyFields(ctx context.Context, table, mutationCode string) ([]ConcurrencyKeyField, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return nil, err
	}
	if protectedTable(table) {
		return nil, ErrProtectedTable
	}
	schema, err := m.metadata.GetTableSchema(ctx, table)
	if err != nil {
		return nil, err
	}
	policy, err := m.mutationPolicies.Get(ctx, mutationCode)
	if err != nil {
		return nil, err
	}
	fields := make([]ConcurrencyKeyField, 0, len(schema.Columns))
	for _, column := range schema.Columns {
		fields = append(fields, ConcurrencyKeyField{Name: column.Name, Type: column.Type, Eligible: ValidateConcurrencyKey(schema, policy, []string{column.Name}) == nil})
	}
	return fields, nil
}

// ValidateConcurrencyKey is shared by assignment validation and live draft
// preparation: a policy or schema change cannot make a deferred value a target.
func ValidateConcurrencyKey(schema domain.TableSchema, policy domain.MutationPolicy, fields []string) error {
	seen := map[string]bool{}
	for _, name := range fields {
		column, found := schema.Column(name)
		if !found || seen[name] || column.Generated || column.AutoIncrement || column.Type == domain.ColumnTypeUnsupported || column.Type == domain.ColumnTypeJSON {
			return ErrConcurrencyKeyInvalid
		}
		for _, automatic := range []*string{policy.CreateOperatorField, policy.CreateTimeField, policy.ModifyOperatorField, policy.ModifyTimeField} {
			if automatic != nil && *automatic == name {
				return ErrConcurrencyKeyInvalid
			}
		}
		seen[name] = true
	}
	return nil
}

func prepareConcurrencyTargets(ctx context.Context, s ReleaseOrderSession, snapshot resolvedPolicySnapshot, items []ReleaseItem) error {
	fields := snapshot.tablePolicy.ConcurrencyKey
	if err := ValidateConcurrencyKey(snapshot.schema, snapshot.mutationPolicy, fields); err != nil {
		return err
	}
	if len(fields) == 0 {
		return nil
	}
	var rows []domain.Row
	var owners []int
	for index, item := range items {
		if item.Operation != "ADD" {
			rows = append(rows, item.Before)
			owners = append(owners, index)
		}
		if item.Operation == "DELETE" {
			continue
		}
		proposed := domain.Row{}
		for _, name := range fields {
			value, supplied := item.Content[name]
			if !supplied && item.Operation == "ADD" {
				return &ReleaseItemError{Index: index, Cause: ErrConcurrencyKeyValue}
			}
			if !supplied {
				value = item.Before[name]
			}
			proposed[name] = value
		}
		rows = append(rows, proposed)
		owners = append(owners, index)
	}
	keys, err := s.ReadConcurrencyKeys(ctx, snapshot.schema, fields, rows)
	if err != nil {
		var itemError *ReleaseItemError
		if errors.As(err, &itemError) && itemError.Index >= 0 && itemError.Index < len(owners) {
			itemError.Index = owners[itemError.Index]
		}
		return err
	}
	for index, key := range keys {
		items[owners[index]].ConcurrencyKeys = append(items[owners[index]].ConcurrencyKeys, key)
	}
	return nil
}
