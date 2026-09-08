package application

import (
	"context"
	"encoding/hex"
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
	CommitPublication(context.Context, PublicationPlan) (domain.PublicationResult, error)
}
type PublicationPlan struct {
	OrderID, PublisherID, SchemaDigest string
	At                                 time.Time
	Schema                             domain.TableSchema
	Execution                          domain.TableExecutionSchema
	Policy                             domain.MutationPolicy
	Items                              []PublicationItem
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
	return r.changeOrderUsing(ctx, id, input.ExpectedVersion, "execute", key, input, execute, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		schema, err := s.LockAndReadTableExecutionSchema(ctx, order.TableName)
		if err != nil {
			return err
		}
		if err = publication.LockPublicationTable(ctx, schema.TableName); err != nil {
			return err
		}
		snapshot, err := r.snapshots.resolve(ctx, s, order.TableName, mutationPolicySnapshot)
		if err != nil {
			return err
		}
		p := snapshot.mutationPolicy
		current := domain.ReleaseExecutionSnapshot{Schema: schema, Mutation: domain.NewReleaseMutationSemantics(p)}
		if order.Frozen == nil || hex.EncodeToString(releaseDigest(current)) != hex.EncodeToString(releaseDigest(order.Frozen)) {
			return ErrReleaseFrozenChanged
		}
		digest := hex.EncodeToString(releaseDigest(struct {
			Title     string
			Items     []ReleaseItem
			Execution *domain.ReleaseExecutionSnapshot
		}{order.Title, order.Items, order.Frozen}))
		if digest != order.FrozenDigest {
			return ErrReleaseFrozenChanged
		}
		prepared, err := r.prepareOrder(ctx, s, *order)
		if err != nil {
			return err
		}
		if hex.EncodeToString(releaseDigest(prepared)) != hex.EncodeToString(releaseDigest(order.Items)) {
			return ErrRecordVersionConflict
		}
		now, err := s.DatabaseTime(ctx)
		if err != nil {
			return err
		}
		plan := PublicationPlan{OrderID: order.ID, PublisherID: actor, At: now, Schema: snapshot.schema, SchemaDigest: hex.EncodeToString(releaseDigest(schema)), Execution: schema, Policy: p}
		idColumn, _ := snapshot.schema.Column("id")
		for _, item := range order.Items {
			entry := PublicationItem{Intent: item}
			if item.ID != nil {
				entry.ID, err = domain.ParseColumnValue(idColumn, *item.ID)
				if err != nil {
					return ErrInvalidMutation
				}
			}
			content, err := publicationContent(snapshot.schema, p, item, actor, now)
			if err != nil {
				return err
			}
			entry.Values, err = releaseMutationValues(snapshot.schema, content, item.Operation == "ADD", order.RollbackOfID != "")
			if err != nil {
				return err
			}
			plan.Items = append(plan.Items, entry)
		}
		result, err := publication.CommitPublication(ctx, plan)
		if err != nil {
			return err
		}
		if order.RollbackOfID != "" {
			original, err := s.GetReleaseOrder(ctx, order.RollbackOfID)
			if err != nil {
				return err
			}
			if err := verifyRollbackResult(original, result, snapshot.schema, p); err != nil {
				return err
			}
		}
		order.Publication = &result
		order.State = "SUCCEEDED"
		if order.RollbackOfID != "" {
			order.State = "COMPLETED"
		}
		if err := r.finishRollback(ctx, s, *order, true); err != nil {
			return err
		}
		if order.RollbackOfID != "" {
			return s.ReleaseTargets(ctx, order.ID)
		}
		return nil
	})
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
