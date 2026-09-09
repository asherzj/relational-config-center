package application

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// QuickRollbackPreview is a whole-order restoration review, not a saved order
// or approval. The digest binds its current intent and execution semantics.
type QuickRollbackPreview struct {
	OrderID         string        `json:"order_id"`
	ExpectedVersion string        `json:"expected_version"`
	TableName       string        `json:"table_name"`
	PreviewDigest   string        `json:"preview_digest"`
	Items           []ReleaseItem `json:"items"`
}

func (r *ReleaseOrders) PreviewQuickRollback(ctx context.Context, id string, input SubmitReleaseInput) (QuickRollbackPreview, error) {
	if _, err := requireRole(ctx, RolePublisher); err != nil {
		return QuickRollbackPreview{}, err
	}
	var result QuickRollbackPreview
	err := r.store.ExecutePublication(ctx, func(s PublicationSession) error {
		original, err := s.GetReleaseOrder(ctx, id)
		if err != nil {
			return err
		}
		result, _, err = r.prepareQuickRollback(ctx, s, original, input.ExpectedVersion)
		return err
	})
	return result, err
}

func (r *ReleaseOrders) prepareQuickRollback(ctx context.Context, s PublicationSession, original ReleaseOrder, version string) (QuickRollbackPreview, *resolvedPolicySnapshot, error) {
	fail := func(err error) (QuickRollbackPreview, *resolvedPolicySnapshot, error) {
		return QuickRollbackPreview{}, nil, err
	}
	if ValidateRecordVersion(version) != nil || version == "0" {
		return fail(ErrReleaseInvalid)
	}
	if original.Version != version {
		return fail(ErrReleaseVersionConflict)
	}
	if original.State != "SUCCEEDED" || original.RollbackOfID != "" || original.RollbackPending {
		return fail(ErrReleaseState)
	}
	if original.Publication == nil || original.VerifyPublication() != nil {
		return fail(ErrReleaseUnavailable)
	}
	if err := s.LockPublicationTable(ctx, original.TableName); err != nil {
		return fail(err)
	}
	schema, err := s.LockAndReadTableExecutionSchema(ctx, original.TableName)
	if err != nil {
		return fail(err)
	}
	snapshot, err := r.snapshots.resolve(ctx, s, original.TableName, mutationPolicySnapshot)
	if err != nil {
		return fail(err)
	}
	current := domain.ReleaseExecutionSnapshot{Schema: schema, Mutation: domain.NewReleaseMutationSemantics(snapshot.mutationPolicy)}
	if hex.EncodeToString(releaseDigest(current)) != hex.EncodeToString(releaseDigest(original.Frozen)) {
		return fail(ErrReleaseFrozenChanged)
	}
	if err := s.LockUnchangedPublication(ctx, original, snapshot.schema); err != nil {
		return fail(err)
	}
	items, err := r.reverseItems(ctx, s, original)
	if err != nil {
		return fail(err)
	}
	result := QuickRollbackPreview{OrderID: original.ID, ExpectedVersion: version, TableName: original.TableName, Items: items}
	result.PreviewDigest = hex.EncodeToString(releaseDigest(struct {
		Preview   QuickRollbackPreview
		Execution domain.ReleaseExecutionSnapshot
	}{result, current}))
	return result, &snapshot, nil
}

// QuickRollbackInput must survive an unknown response unchanged. A new preview
// cannot silently replace the reviewed digest of a possibly committed request.
type QuickRollbackInput struct {
	ExpectedVersion string `json:"expected_version"`
	PreviewDigest   string `json:"preview_digest"`
	Reason          string `json:"reason"`
}

func (r *ReleaseOrders) QuickRollback(ctx context.Context, id string, input QuickRollbackInput, key string) (ReleaseOrder, error) {
	actor, err := requireRole(ctx, RolePublisher)
	if err != nil {
		return ReleaseOrder{}, err
	}
	if !roleRequestKey.MatchString(key) {
		return ReleaseOrder{}, ErrReleaseInvalid
	}
	var result ReleaseOrder
	err = r.store.ExecutePublication(ctx, func(s PublicationSession) error {
		operation := "quick-rollback:" + id
		previous, err := s.BeginReleaseRequest(ctx, actor, operation, key, releaseDigest(input))
		if err != nil {
			return err
		}
		original, err := s.GetReleaseOrder(ctx, id)
		if err != nil {
			return err
		}
		if previous != nil {
			result = *previous
			return nil
		}
		digest, err := hex.DecodeString(input.PreviewDigest)
		if err != nil || len(digest) != 32 || len(input.Reason) > 2000 {
			return ErrReleaseInvalid
		}
		preview, snapshot, err := r.prepareQuickRollback(ctx, s, original, input.ExpectedVersion)
		if err != nil {
			return err
		}
		if preview.PreviewDigest != input.PreviewDigest {
			return ErrReleaseFrozenChanged
		}
		now, err := s.DatabaseTime(ctx)
		if err != nil {
			return err
		}
		stamp := now.UTC().Format(time.RFC3339Nano)
		plan := PublicationPlan{ExecutionKind: "ROLLBACK", OrderID: original.ID, TargetOrderID: original.ID, PublisherID: actor, At: now, Schema: snapshot.schema, SchemaDigest: hex.EncodeToString(releaseDigest(original.Frozen.Schema)), Execution: original.Frozen.Schema, Policy: snapshot.mutationPolicy}
		idColumn, _ := snapshot.schema.Column("id")
		for _, item := range preview.Items {
			entry := PublicationItem{Intent: item}
			if item.ID == nil {
				return ErrReleaseUnavailable
			}
			entry.ID, err = domain.ParseColumnValue(idColumn, *item.ID)
			if err != nil {
				return ErrInvalidMutation
			}
			content, err := publicationContent(snapshot.schema, snapshot.mutationPolicy, item, actor, now)
			if err != nil {
				return err
			}
			entry.Values, err = releaseMutationValues(snapshot.schema, content, item.Operation == "ADD", true)
			if err != nil {
				return err
			}
			plan.Items = append(plan.Items, entry)
		}
		publication, err := s.CommitPublication(ctx, plan)
		if err != nil {
			return err
		}
		if err = verifyRollbackResult(original, publication, snapshot.schema, snapshot.mutationPolicy); err != nil {
			return err
		}
		original.Rollback = &publication
		original.Executions = append(original.Executions, domain.SummarizeExecution(publication))
		original.State = "ROLLED_BACK"
		if err = appendRelatedReleaseEvent(&original, actor, stamp, "QUICK_ROLLBACK", input.Reason, ""); err != nil {
			return err
		}
		if err = s.SaveReleaseOrder(ctx, original, false); err != nil {
			return err
		}
		result = original
		if err = s.ReleaseTargets(ctx, original.ID); err != nil {
			return err
		}
		return s.CompleteReleaseRequest(ctx, actor, operation, key, result)
	})
	return result, err
}
