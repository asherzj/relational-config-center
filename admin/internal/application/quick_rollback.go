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

func (r *ReleaseOrders) prepareQuickRollback(ctx context.Context, s PublicationSession, original ReleaseOrder, version string) (QuickRollbackPreview, map[string]PublicationTable, error) {
	fail := func(err error) (QuickRollbackPreview, map[string]PublicationTable, error) {
		return QuickRollbackPreview{}, nil, err
	}
	if ValidateRecordVersion(version) != nil || version == "0" {
		return fail(ErrReleaseInvalid)
	}
	if original.Version != version {
		return fail(ErrReleaseVersionConflict)
	}
	if original.State != "SUCCEEDED" {
		return fail(ErrReleaseState)
	}
	if original.VerifyPublication() != nil {
		return fail(ErrReleaseUnavailable)
	}
	tables, err := r.resolveReleaseTables(ctx, s, original.Items, true)
	if err != nil {
		return fail(ReverseReleaseItemError(err, len(original.Items)))
	}
	if err := verifyFrozenTables(original, tables); err != nil {
		return fail(ReverseReleaseItemError(err, len(original.Items)))
	}
	if err := s.LockUnchangedPublication(ctx, original, tables); err != nil {
		return fail(err)
	}
	items, err := r.reverseItems(ctx, s, original)
	if err != nil {
		return fail(err)
	}
	result := QuickRollbackPreview{OrderID: original.ID, ExpectedVersion: version, Items: items}
	result.PreviewDigest = hex.EncodeToString(releaseDigest(struct {
		Preview   QuickRollbackPreview
		Execution map[string]domain.ReleaseExecutionSnapshot
	}{result, frozenReleaseTables(tables)}))
	return result, tables, nil
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
		preview, tables, err := r.prepareQuickRollback(ctx, s, original, input.ExpectedVersion)
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
		plan, err := buildPublicationPlan(original, "ROLLBACK", preview.Items, tables, actor, now)
		if err != nil {
			return err
		}
		publication, err := s.CommitPublication(ctx, plan)
		if err != nil {
			return err
		}
		if err = verifyRollbackResult(original, publication, tables); err != nil {
			return err
		}
		if err := original.ApplyExecution(publication); err != nil {
			return ErrReleaseUnavailable
		}
		original.State = "ROLLED_BACK"
		if err = appendRelatedReleaseEvent(&original, actor, stamp, "QUICK_ROLLBACK", input.Reason, ""); err != nil {
			return err
		}
		original.History[len(original.History)-1].ExecutionID = publication.ExecutionID
		advanceReleaseFlows(&original)
		if err = s.SaveReleaseOrder(ctx, original, false); err != nil {
			return err
		}
		result = original
		if err = s.ReleaseTargets(ctx, original.ID); err != nil {
			return err
		}
		if err = s.RecordApprovalNotifications(ctx, original, actor, releaseResultRecipients(original)); err != nil {
			return err
		}
		return s.CompleteReleaseRequest(ctx, actor, operation, key, result)
	})
	return result, r.recordExecutionFailure(ctx, releaseExecutionAttempt{OrderID: id, ActorID: actor, Operation: "quick-rollback", Key: key, ExpectedVersion: input.ExpectedVersion, Input: input}, err)
}
