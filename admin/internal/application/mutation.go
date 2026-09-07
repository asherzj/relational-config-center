package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrOperatorFieldIncompatible = errors.New("operator field cannot store a complete Account ID")
	ErrMutationNotAllowed        = errors.New("mutation operation is not allowed")
	ErrInvalidMutation           = errors.New("invalid mutation content")
	ErrMissingRequiredField      = errors.New("required mutation field is missing")
	ErrDuplicateKey              = errors.New("duplicate key")
	ErrMutationRowNotFound       = errors.New("mutation row not found")
	ErrMutationUnavailable       = errors.New("mutation unavailable")
	ErrMutationTimeout           = errors.New("mutation timeout")
)

// MutationExecutor is the single deep execution interface exposed by the
// Mutation module. GORM, dynamic clauses, transactions, affected-row rules,
// duplicate-key inspection, and LastInsertId remain inside its MySQL adapter.
type MutationExecutor interface {
	InsertRow(context.Context, domain.RowInsert) (string, error)
	UpdateRow(context.Context, domain.RowUpdate) (int64, error)
	DeleteRow(context.Context, domain.RowDelete) (int64, error)
}

// MutationSnapshotSession is the request-scoped, read-write view of the
// Managed Data Source. Catalog aggregates remain separate operations and the
// governed row write uses this same session; implementations must reject use
// after ExecuteMutationSnapshot returns.
type MutationSnapshotSession interface {
	PolicySnapshotReader
	MutationExecutor
	DatabaseTime(context.Context) (time.Time, error)
}

// MutationSnapshotExecutor owns the single read-write REPEATABLE READ
// transaction containing Policy Snapshot resolution, live Schema validation,
// authorization, Auto Fill production, and the Managed Table row change.
type MutationSnapshotExecutor interface {
	ExecuteMutationSnapshot(context.Context, func(MutationSnapshotSession) error) error
}

// ManagedTableMutation owns Policy Snapshot loading, live Schema validation,
// and fresh Mutation Policy construction for every request.
type ManagedTableMutation struct {
	snapshotExecutor MutationSnapshotExecutor
	snapshots        *policySnapshotResolver
}

func NewManagedTableMutation(executor MutationSnapshotExecutor, queryTypes *QueryPolicyTypeRegistry, mutationTypes *MutationPolicyTypeRegistry) *ManagedTableMutation {
	return &ManagedTableMutation{snapshotExecutor: executor, snapshots: newPolicySnapshotResolver(queryTypes, mutationTypes)}
}

func (mutation *ManagedTableMutation) Add(ctx context.Context, tableName string, content domain.MutationContent) (string, error) {
	if _, err := requireRole(ctx, RoleEditor); err != nil {
		return "", err
	}
	if protectedTable(tableName) {
		return "", ErrProtectedTable
	}
	var id string
	err := mutation.snapshotExecutor.ExecuteMutationSnapshot(ctx, func(session MutationSnapshotSession) error {
		snapshot, err := mutation.snapshots.resolve(ctx, session, tableName, mutationPolicySnapshot)
		if err != nil {
			return err
		}
		if !snapshot.mutationPolicy.AllowAdd {
			return ErrMutationNotAllowed
		}
		id, err = mutation.relationalAdd(ctx, session, snapshot.schema, snapshot.mutationPolicy, content)
		return err
	})
	return id, err
}

func (mutation *ManagedTableMutation) Modify(ctx context.Context, tableName string, id domain.JSONString, content domain.MutationContent) (int64, error) {
	if _, err := requireRole(ctx, RoleEditor); err != nil {
		return 0, err
	}
	if protectedTable(tableName) {
		return 0, ErrProtectedTable
	}
	var affected int64
	err := mutation.snapshotExecutor.ExecuteMutationSnapshot(ctx, func(session MutationSnapshotSession) error {
		snapshot, err := mutation.snapshots.resolve(ctx, session, tableName, mutationPolicySnapshot)
		if err != nil {
			return err
		}
		if !snapshot.mutationPolicy.AllowModify {
			return ErrMutationNotAllowed
		}
		affected, err = mutation.relationalModify(ctx, session, snapshot.schema, snapshot.mutationPolicy, id, content)
		return err
	})
	return affected, err
}

func (mutation *ManagedTableMutation) Delete(ctx context.Context, tableName string, id domain.JSONString) (int64, error) {
	if _, err := requireRole(ctx, RoleEditor); err != nil {
		return 0, err
	}
	if protectedTable(tableName) {
		return 0, ErrProtectedTable
	}
	var affected int64
	err := mutation.snapshotExecutor.ExecuteMutationSnapshot(ctx, func(session MutationSnapshotSession) error {
		snapshot, err := mutation.snapshots.resolve(ctx, session, tableName, mutationPolicySnapshot)
		if err != nil {
			return err
		}
		if !snapshot.mutationPolicy.AllowDelete {
			return ErrMutationNotAllowed
		}
		affected, err = relationalDelete(ctx, session, snapshot.schema, id)
		return err
	})
	return affected, err
}

func (mutation *ManagedTableMutation) relationalAdd(ctx context.Context, session MutationSnapshotSession, schema domain.TableSchema, policy domain.MutationPolicy, content domain.MutationContent) (string, error) {
	effective, err := mutation.effectiveRelationalContent(ctx, session, schema, policy, MutationOperationAdd, content)
	if err != nil {
		return "", err
	}
	values, err := mutationValues(schema, effective, true)
	if err != nil {
		return "", err
	}
	for _, column := range schema.Columns {
		if _, supplied := effective[column.Name]; column.RequiredForInsert() && !supplied {
			return "", ErrMissingRequiredField
		}
	}
	return session.InsertRow(ctx, domain.RowInsert{TableName: schema.Name, Values: values, ProvidedID: effective["id"]})
}

func (mutation *ManagedTableMutation) relationalModify(ctx context.Context, session MutationSnapshotSession, schema domain.TableSchema, policy domain.MutationPolicy, id domain.JSONString, content domain.MutationContent) (int64, error) {
	if _, supplied := content["id"]; supplied {
		return 0, ErrInvalidMutation
	}
	idColumn, found := schema.Column("id")
	if !found || idColumn.Type == domain.ColumnTypeUnsupported {
		return 0, ErrIncompatibleTable
	}
	parsedID, err := domain.ParseColumnValue(idColumn, id)
	if err != nil {
		return 0, ErrInvalidMutation
	}
	effective, err := mutation.effectiveRelationalContent(ctx, session, schema, policy, MutationOperationModify, content)
	if err != nil {
		return 0, err
	}
	if len(effective) == 0 {
		return 0, ErrInvalidMutation
	}
	values, err := mutationValues(schema, effective, false)
	if err != nil {
		return 0, err
	}
	return session.UpdateRow(ctx, domain.RowUpdate{TableName: schema.Name, IDColumn: idColumn, ID: parsedID, Values: values})
}

func relationalDelete(ctx context.Context, session MutationSnapshotSession, schema domain.TableSchema, id domain.JSONString) (int64, error) {
	idColumn, found := schema.Column("id")
	if !found || idColumn.Type == domain.ColumnTypeUnsupported {
		return 0, ErrIncompatibleTable
	}
	parsedID, err := domain.ParseColumnValue(idColumn, id)
	if err != nil {
		return 0, ErrInvalidMutation
	}
	return session.DeleteRow(ctx, domain.RowDelete{TableName: schema.Name, IDColumn: idColumn, ID: parsedID})
}

func (mutation *ManagedTableMutation) effectiveRelationalContent(ctx context.Context, session MutationSnapshotSession, schema domain.TableSchema, policy domain.MutationPolicy, operation MutationOperation, content domain.MutationContent) (domain.MutationContent, error) {
	for _, field := range []*string{policy.CreateOperatorField, policy.CreateTimeField, policy.ModifyOperatorField, policy.ModifyTimeField} {
		if field != nil {
			if _, supplied := content[*field]; supplied {
				return nil, ErrInvalidMutation
			}
		}
	}
	effective := make(domain.MutationContent, len(content)+4)
	for field, value := range content {
		effective[field] = value
	}

	operatorFields := make([]*string, 0, 2)
	timeFields := make([]*string, 0, 2)
	if operation == MutationOperationAdd {
		operatorFields = append(operatorFields, policy.CreateOperatorField)
		timeFields = append(timeFields, policy.CreateTimeField)
	}
	if operation == MutationOperationAdd || operation == MutationOperationModify {
		operatorFields = append(operatorFields, policy.ModifyOperatorField)
		timeFields = append(timeFields, policy.ModifyTimeField)
	}

	if nonNilFieldCount(operatorFields) > 0 {
		operator, err := requireRole(ctx, RoleEditor)
		if err != nil {
			return nil, err
		}
		for _, field := range operatorFields {
			if field != nil {
				column, found := schema.Column(*field)
				if !found || column.Type != domain.ColumnTypeString || column.TextCapacity < 36 {
					return nil, ErrOperatorFieldIncompatible
				}
				value := domain.JSONString(operator)
				effective[*field] = &value
			}
		}
	}
	if nonNilFieldCount(timeFields) > 0 {
		now, err := session.DatabaseTime(ctx)
		if err != nil {
			return nil, err
		}
		for _, field := range timeFields {
			if field == nil {
				continue
			}
			column, found := schema.Column(*field)
			if !found {
				return nil, ErrInvalidMutationPolicyRules
			}
			value, err := currentTimeValue(column, now)
			if err != nil {
				return nil, ErrInvalidMutationPolicyRules
			}
			effective[*field] = &value
		}
	}
	return effective, nil
}

func nonNilFieldCount(fields []*string) int {
	count := 0
	for _, field := range fields {
		if field != nil {
			count++
		}
	}
	return count
}

func currentTimeValue(column domain.Column, now time.Time) (domain.JSONString, error) {
	now = now.Truncate(time.Microsecond)
	switch column.Type {
	case domain.ColumnTypeDate:
		return domain.JSONString(now.Format("2006-01-02")), nil
	case domain.ColumnTypeTime:
		return domain.JSONString(formatAutoFillTime(now, "15:04:05", "")), nil
	case domain.ColumnTypeDateTime:
		return domain.JSONString(formatAutoFillTime(now, "2006-01-02 15:04:05", "")), nil
	case domain.ColumnTypeTimestamp:
		return domain.JSONString(formatAutoFillTime(now, "2006-01-02T15:04:05", "Z")), nil
	default:
		return "", ErrInvalidMutation
	}
}

func formatAutoFillTime(value time.Time, layout, suffix string) string {
	formatted := value.Format(layout)
	if microseconds := value.Nanosecond() / 1000; microseconds != 0 {
		formatted += "." + strings.TrimRight(fmt.Sprintf("%06d", microseconds), "0")
	}
	return formatted + suffix
}

func mutationValues(schema domain.TableSchema, content domain.MutationContent, allowID bool) ([]domain.MutationValue, error) {
	values := make([]domain.MutationValue, 0, len(content))
	for field, value := range content {
		if field == "id" && !allowID {
			return nil, ErrInvalidMutation
		}
		column, found := schema.Column(field)
		if !found || !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
			return nil, ErrInvalidMutation
		}
		if value == nil {
			if !column.Nullable {
				return nil, ErrInvalidMutation
			}
			values = append(values, domain.MutationValue{Column: column})
			continue
		}
		parsed, err := domain.ParseColumnValue(column, *value)
		if err != nil {
			return nil, ErrInvalidMutation
		}
		values = append(values, domain.MutationValue{Column: column, Value: parsed})
	}
	return values, nil
}
