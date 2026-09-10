package application

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// RollbackReasonInput is intentionally independent of the Release Order CAS.
// A correction appends audit state without reopening or revising the workflow.
type RollbackReasonInput struct {
	Reason string `json:"reason"`
}

func (r *ReleaseOrders) ChangeRollbackReason(ctx context.Context, id string, input RollbackReasonInput, key string) (ReleaseOrder, error) {
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return ReleaseOrder{}, err
	}
	if !roleRequestKey.MatchString(key) || !utf8.ValidString(input.Reason) || len(input.Reason) > 2000 {
		return ReleaseOrder{}, ErrReleaseInvalid
	}
	var result ReleaseOrder
	err = r.store.ExecuteReleaseOrder(ctx, func(s ReleaseOrderSession) error {
		operation := "rollback-reason:" + id
		previous, err := s.BeginReleaseRequest(ctx, actor, operation, key, releaseDigest(input))
		if err != nil {
			return err
		}
		order, err := s.GetReleaseOrder(ctx, id)
		if err != nil {
			return err
		}
		if !releaseOrderActionState(order, "edit-rollback-reason") {
			return ErrReleaseState
		}
		if err := authorizeReleaseAction(ctx, order, "edit-rollback-reason"); err != nil {
			return err
		}
		if previous != nil {
			result = *previous
			return nil
		}
		execution, found := rollbackExecution(order)
		if !found {
			return ErrReleaseUnavailable
		}
		now, err := s.DatabaseTime(ctx)
		if err != nil {
			return err
		}
		event := domain.ReleaseEvent{
			ExecutionID: execution.ID,
			Action:      "ROLLBACK_REASON",
			ActorID:     actor,
			At:          now.UTC().Format(time.RFC3339Nano),
			Version:     order.Version,
			Reason:      input.Reason,
		}
		if err := s.AppendRollbackReason(ctx, order.ID, event); err != nil {
			return err
		}
		order.History = append(order.History, event)
		result = order
		return s.CompleteReleaseRequest(ctx, actor, operation, key, result)
	})
	return result, err
}
