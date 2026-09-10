package application

import (
	"context"
	"errors"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// ReleaseExecutionFailure reports a known uncommitted business attempt and
// separately states whether its operation history could be persisted.
type ReleaseExecutionFailure struct {
	Cause        error
	HistorySaved bool
}

func (e *ReleaseExecutionFailure) Error() string { return e.Cause.Error() }
func (e *ReleaseExecutionFailure) Unwrap() error { return e.Cause }

type releaseExecutionAttempt struct {
	OrderID, ActorID, Operation, Key string
	ExpectedVersion                  string
	Input                            any
}

func (r *ReleaseOrders) recordExecutionFailure(ctx context.Context, attempt releaseExecutionAttempt, cause error) error {
	if cause == nil {
		return nil
	}
	// Request/authority rejections are not execution attempts. A missing COMMIT
	// acknowledgement is never evidence that the execution failed.
	for _, excluded := range []error{ErrReleaseUnknown, ErrPermissionDenied, ErrSession, ErrReleaseInvalid, ErrReleaseNotFound, ErrReleaseVersionConflict, ErrReleaseState, ErrReleaseIdempotencyConflict} {
		if errors.Is(cause, excluded) {
			return cause
		}
	}
	// The business transaction has already returned without committing. Audit
	// may outlive the HTTP deadline, but is bounded and never repeats the write.
	audit, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	action := "EXECUTE_FAILED"
	if attempt.Operation == "quick-rollback" {
		action = "QUICK_ROLLBACK_FAILED"
	}
	event := domain.ReleaseEvent{Action: action, ActorID: attempt.ActorID, Version: attempt.ExpectedVersion, Reason: "本次执行未提交，未产生业务变更或成功执行记录。"}
	err := r.store.ExecuteReleaseOrder(audit, func(s ReleaseOrderSession) error {
		if _, err := s.BeginReleaseRequest(audit, attempt.ActorID, attempt.Operation+":"+attempt.OrderID, attempt.Key, releaseDigest(attempt.Input)); err != nil {
			return err
		}
		return s.AppendReleaseFailure(audit, attempt.OrderID, event)
	})
	return &ReleaseExecutionFailure{Cause: cause, HistorySaved: err == nil}
}
