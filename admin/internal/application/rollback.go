package application

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrRollbackConflict        = errors.New("publication already has an active rollback")
	ErrRollbackLocked          = errors.New("rollback intent cannot be edited or copied")
	ErrRollbackRestoreMismatch = errors.New("original business values cannot be restored")
)

// Rollback's former separate-order route is retired. The only restoration is
// the publisher's reviewed QuickRollback of an unfinished original order.
func (r *ReleaseOrders) Rollback(ctx context.Context, id string, input CancelReleaseInput, key string) (ReleaseOrder, error) {
	if _, err := requireRole(ctx, RolePublisher); err != nil {
		return ReleaseOrder{}, err
	}
	return ReleaseOrder{}, ErrReleaseState
}

func (r *ReleaseOrders) prepareOrder(ctx context.Context, s ReleaseOrderSession, order ReleaseOrder) ([]ReleaseItem, error) {
	if order.RollbackOfID != "" {
		original, err := s.GetReleaseOrder(ctx, order.RollbackOfID)
		if err != nil {
			return nil, err
		}
		if original.State != "COMPLETED" || !original.RollbackPending || original.RollbackOrderID != order.ID {
			return nil, ErrRollbackConflict
		}
		return r.reverseItems(ctx, s, original)
	}
	input := DraftInput{TableName: order.TableName}
	for _, item := range order.Items {
		entry := DraftItemInput{DetailID: item.DetailID, TableName: item.TableName, Operation: item.Operation, ID: item.ID, ExpectedRecordVersion: item.ExpectedRecordVersion, Content: item.Content}
		if entry.Operation == "ADD" {
			entry.ID = nil
		}
		input.Items = append(input.Items, entry)
	}
	return r.prepare(ctx, s, input, false)
}

func (r *ReleaseOrders) reverseItems(ctx context.Context, s ReleaseOrderSession, original ReleaseOrder) ([]ReleaseItem, error) {
	if original.Publication == nil || original.VerifyPublication() != nil {
		return nil, ErrReleaseUnavailable
	}
	// Callers already acquired every table guard before establishing a snapshot.
	snapshots := map[string]resolvedPolicySnapshot{}
	for _, table := range releaseTableNames(original.Items) {
		snapshot, err := r.snapshots.resolve(ctx, s, table, mutationPolicySnapshot)
		if err != nil {
			return nil, err
		}
		snapshots[table] = snapshot
	}
	input := DraftInput{TableName: original.TableName}
	// Unwind the actual DML order so later items release any unique values
	// before earlier items restore them. Error indexes belong to this new order.
	for index := range original.Publication.Commands {
		command := original.Publication.Commands[len(original.Publication.Commands)-1-index]
		snapshot := snapshots[command.TableName]
		reserved := rollbackDeferredFields(original.FrozenTables[command.TableName], snapshot.mutationPolicy)
		id := domain.JSONString(command.ID)
		source := original.Items[len(original.Items)-1-index]
		item := DraftItemInput{DetailID: source.DetailID, TableName: source.TableName, ID: &id, ExpectedRecordVersion: command.RecordVersion, Content: MutationContent{}}
		switch command.Operation {
		case "ADD":
			item.Operation = "DELETE"
		case "MODIFY":
			item.Operation = "MODIFY"
		case "DELETE":
			item.Operation = "ADD"
			item.ID = nil
		default:
			return nil, ErrReleaseUnavailable
		}
		if item.Operation != "DELETE" {
			for _, field := range command.Before.Fields {
				column, exists := snapshot.schema.Column(field.Name)
				if reserved[field.Name] || field.Name == "id" && item.Operation == "MODIFY" {
					continue
				}
				if !exists {
					return nil, &ReleaseItemError{Index: index, Cause: ErrRollbackRestoreMismatch}
				}
				if column.Generated {
					continue
				}
				value, err := rollbackFieldValue(field)
				if err != nil {
					return nil, &ReleaseItemError{Index: index, Cause: err}
				}
				item.Content[field.Name] = value
			}
		}
		input.Items = append(input.Items, item)
	}
	// These values were persisted by a verified publication, not uploaded by this
	// small action request. Live schema/policy and complete-document budgets apply.
	return r.prepareInput(ctx, s, input, false, &original)
}

func rollbackFieldValue(field domain.CanonicalField) (*domain.JSONString, error) {
	if field.Encoding == "sql_null" {
		return nil, nil
	}
	if field.Value == nil || field.Encoding != "text" && field.Encoding != "json" {
		return nil, ErrRollbackRestoreMismatch
	}
	value := *field.Value
	if strings.HasPrefix(strings.ToLower(field.Type), "timestamp") {
		value = strings.Replace(value, " ", "T", 1) + "Z"
	}
	result := domain.JSONString(value)
	return &result, nil
}

func rollbackDeferredFields(frozen domain.ReleaseExecutionSnapshot, current domain.MutationPolicy) map[string]bool {
	fields := map[string]bool{}
	for _, p := range []domain.ReleaseMutationSemantics{frozen.Mutation, domain.NewReleaseMutationSemantics(current)} {
		for _, name := range []*string{p.CreateOperatorField, p.CreateTimeField, p.ModifyOperatorField, p.ModifyTimeField} {
			if name != nil {
				fields[*name] = true
			}
		}
	}
	// VerifyPublication has already validated this frozen column projection.
	columns, _ := frozen.Schema.Columns()
	for _, column := range columns {
		if column.GenerationExpression != nil && *column.GenerationExpression != "" {
			fields[column.Name] = true
		}
	}
	return fields
}

func verifyRollbackResult(original ReleaseOrder, result domain.PublicationResult, tables map[string]PublicationTable) error {
	if original.Publication == nil || len(original.Publication.Commands) != len(result.Commands) {
		return ErrReleaseUnavailable
	}
	for index, actual := range result.Commands {
		source := original.Publication.Commands[len(original.Publication.Commands)-1-index]
		table := tables[source.TableName]
		deferred := rollbackDeferredFields(original.FrozenTables[source.TableName], table.Policy)
		if actual.TableName != source.TableName || actual.ID != source.ID || actual.Final.Deleted != source.Before.Deleted {
			return &ReleaseItemError{Index: index, Cause: ErrRollbackRestoreMismatch}
		}
		fields := map[string]domain.CanonicalField{}
		for _, field := range actual.Final.Fields {
			fields[field.Name] = field
		}
		for _, expected := range source.Before.Fields {
			column, exists := table.Schema.Column(expected.Name)
			if deferred[expected.Name] || exists && column.Generated {
				continue
			}
			field, found := fields[expected.Name]
			if !found || field.Encoding != expected.Encoding || (field.Value == nil) != (expected.Value == nil) || field.Value != nil && *field.Value != *expected.Value {
				return &ReleaseItemError{Index: index, Cause: ErrRollbackRestoreMismatch}
			}
		}
	}
	return nil
}

func appendRelatedReleaseEvent(order *ReleaseOrder, actor, stamp, action, reason, related string) error {
	version, err := strconv.ParseUint(order.Version, 10, 64)
	if err != nil || version == math.MaxUint64 {
		return ErrReleaseVersionConflict
	}
	order.Version, order.UpdatedAt = strconv.FormatUint(version+1, 10), stamp
	order.History = append(order.History, domain.ReleaseEvent{Action: action, ActorID: actor, At: stamp, Version: order.Version, Reason: reason, RelatedOrderID: related})
	return nil
}

// Linking and closing the original happens inside the same workflow/publication
// transaction as the reverse order, including failure rollback and request result.
func (r *ReleaseOrders) finishRollback(ctx context.Context, s ReleaseOrderSession, order ReleaseOrder, succeeded bool) error {
	if order.RollbackOfID == "" {
		return nil
	}
	original, err := s.GetReleaseOrder(ctx, order.RollbackOfID)
	if err != nil {
		return err
	}
	if original.State != "COMPLETED" || !original.RollbackPending || original.RollbackOrderID != order.ID {
		return ErrRollbackConflict
	}
	original.RollbackPending = false
	action := "ROLLBACK_" + order.State
	if succeeded {
		original.State, action = "ROLLED_BACK", "ROLLED_BACK"
	}
	now, err := s.DatabaseTime(ctx)
	if err != nil {
		return err
	}
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return err
	}
	if err := appendRelatedReleaseEvent(&original, actor, now.UTC().Format(time.RFC3339Nano), action, "", order.ID); err != nil {
		return err
	}
	return s.SaveReleaseOrder(ctx, original, false)
}

// Published MySQL TIME is a signed duration, while uploaded time inputs use the
// existing clock-time contract. Only verified server-held history gets this parser.
var storedTimeDuration = regexp.MustCompile(`^-?(?:[0-9]{2}|[0-7][0-9]{2}|8[0-2][0-9]|83[0-8]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]{1,6})?$`)

func releaseMutationValues(schema domain.TableSchema, content MutationContent, allowID, historical bool) ([]domain.MutationValue, error) {
	if !historical {
		return mutationValues(schema, content, allowID)
	}
	return mutationValuesUsing(schema, content, allowID, func(column domain.Column, value domain.JSONString) (any, error) {
		if column.Type == domain.ColumnTypeTime && storedTimeDuration.MatchString(string(value)) {
			return string(value), nil
		}
		return domain.ParseColumnValue(column, value)
	})
}

func rollbackTitle(title string) string {
	runes := []rune("回滚：" + title)
	if len(runes) > 100 {
		runes = runes[:100]
	}
	return string(runes)
}
