package application

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrReleaseFrozenChanged          = errors.New("frozen execution semantics changed")
	ErrPublicationUnsupported        = errors.New("publication effects cannot be completely tracked")
	ErrPublicationMetadataPermission = errors.New("complete foreign-key metadata requires PROCESS")
)

// The draft/approval session has no row-write capability. Publication exposes
// a single complete commit operation; its adapter shares the workflow transaction.
type PublicationSession interface {
	ReleaseOrderSession
	LockPublicationTable(context.Context, string) error
	LockUnchangedPublication(context.Context, domain.ReleaseOrder, map[string]PublicationTable) error
	CommitPublication(context.Context, PublicationPlan) (domain.PublicationResult, error)
}
type PublicationTable struct {
	Schema       domain.TableSchema
	SchemaDigest string
	Execution    domain.TableExecutionSchema
	Policy       domain.MutationPolicy
}

type PublicationPlan struct {
	OrderID, PublisherID string
	ExecutionKind        string
	TargetOrderID        string
	At                   time.Time
	Tables               map[string]PublicationTable
	Items                []PublicationItem
}
type PublicationItem struct {
	Intent domain.ReleaseItem
	ID     any
	Values []domain.MutationValue
}

func (r *ReleaseOrders) Execute(ctx context.Context, id string, input SubmitReleaseInput, key string) (ReleaseOrder, error) {
	actor, err := requireRole(ctx, RolePublisher)
	if err != nil {
		return ReleaseOrder{}, err
	}
	var publication PublicationSession
	execute := func(ctx context.Context, change func(ReleaseOrderSession) error) error {
		return r.store.ExecutePublication(ctx, func(s PublicationSession) error { publication = s; return change(s) })
	}
	result, err := r.changeOrderUsing(ctx, id, input.ExpectedVersion, "execute", key, input, execute, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		tables, err := r.resolveReleaseTables(ctx, publication, order.Items, true)
		if err != nil {
			return err
		}
		if err := verifyFrozenTables(*order, tables); err != nil {
			return err
		}
		prepared, err := r.prepareOrder(ctx, s, *order)
		if err != nil {
			return err
		}
		for index, item := range prepared {
			if !bytes.Equal(releaseDigest(item), releaseDigest(order.Items[index])) {
				return &ReleaseItemError{Index: index, Cause: ErrRecordVersionConflict}
			}
		}
		now, err := s.DatabaseTime(ctx)
		if err != nil {
			return err
		}
		plan, err := buildPublicationPlan(*order, "PUBLICATION", order.Items, tables, actor, now)
		if err != nil {
			return err
		}
		result, err := publication.CommitPublication(ctx, plan)
		if err != nil {
			return err
		}
		order.Publication = &result
		order.Executions = append(order.Executions, domain.SummarizeExecution(result))
		order.State = "SUCCEEDED"
		return nil
	})
	return result, r.recordExecutionFailure(ctx, releaseExecutionAttempt{OrderID: id, ActorID: actor, Operation: "execute", Key: key, ExpectedVersion: input.ExpectedVersion, Input: input}, err)
}
func publicationContent(schema domain.TableSchema, p domain.MutationPolicy, item ReleaseItem, actor string, now time.Time) (domain.MutationContent, error) {
	content := domain.MutationContent{}
	for name, value := range item.Content {
		content[name] = value
	}
	if item.Operation == "DELETE" {
		return content, nil
	}
	fields := []struct{ operator, stamp *string }{{p.ModifyOperatorField, p.ModifyTimeField}}
	if item.Operation == "ADD" {
		fields = append(fields, struct{ operator, stamp *string }{p.CreateOperatorField, p.CreateTimeField})
	}
	for _, pair := range fields {
		if pair.operator != nil {
			if err := validateOperatorField(schema, *pair.operator); err != nil {
				return nil, err
			}
			v := domain.JSONString(actor)
			content[*pair.operator] = &v
		}
		if pair.stamp != nil {
			col, _ := schema.Column(*pair.stamp)
			v, err := currentTimeValue(col, now)
			if err != nil {
				return nil, err
			}
			content[*pair.stamp] = &v
		}
	}
	return content, nil
}

func buildPublicationPlan(order ReleaseOrder, kind string, items []ReleaseItem, tables map[string]PublicationTable, actor string, now time.Time) (PublicationPlan, error) {
	plan := PublicationPlan{OrderID: order.ID, PublisherID: actor, ExecutionKind: kind, At: now, Tables: tables}
	if kind == "ROLLBACK" {
		plan.TargetOrderID = order.ID
	}
	for index, item := range items {
		table, found := tables[item.TableName]
		if !found {
			return PublicationPlan{}, &ReleaseItemError{Index: index, Cause: ErrReleaseUnavailable}
		}
		entry := PublicationItem{Intent: item}
		var err error
		if item.ID != nil {
			column, _ := table.Schema.Column("id")
			entry.ID, err = domain.ParseColumnValue(column, *item.ID)
			if err != nil {
				return PublicationPlan{}, &ReleaseItemError{Index: index, Cause: ErrInvalidMutation}
			}
		}
		content, err := publicationContent(table.Schema, table.Policy, item, actor, now)
		if err != nil {
			return PublicationPlan{}, &ReleaseItemError{Index: index, Cause: err}
		}
		entry.Values, err = releaseMutationValues(table.Schema, content, item.Operation == "ADD", kind == "ROLLBACK")
		if err != nil {
			return PublicationPlan{}, &ReleaseItemError{Index: index, Cause: err}
		}
		plan.Items = append(plan.Items, entry)
	}
	return plan, nil
}
