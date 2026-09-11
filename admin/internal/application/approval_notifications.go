package application

import (
	"context"
	"slices"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Release results belong to the applicant and the people who actually reviewed
// this order. Current role membership cannot replace these historical identities.
func releaseResultRecipients(order domain.ReleaseOrder) []string {
	recipients := []string{order.ApplicantID}
	for _, approval := range order.Approvals {
		if approval.Decision != nil {
			recipients = append(recipients, approval.Decision.ActorID)
		}
	}
	slices.Sort(recipients)
	return slices.Compact(recipients)
}

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
