package application

import (
	"context"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func (r *ReleaseOrders) NotificationCounts(ctx context.Context) (domain.ApprovalNotificationCounts, error) {
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return domain.ApprovalNotificationCounts{}, err
	}
	return r.store.ReadApprovalNotificationCounts(ctx, actor)
}

func (r *ReleaseOrders) AcknowledgeNotification(ctx context.Context, id, sequence string) (domain.ApprovalNotification, error) {
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return domain.ApprovalNotification{}, err
	}
	if ValidateRecordVersion(sequence) != nil {
		return domain.ApprovalNotification{}, ErrReleaseInvalid
	}
	return r.store.AcknowledgeApprovalNotification(ctx, actor, id, sequence)
}
