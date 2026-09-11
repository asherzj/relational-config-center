package application

import (
	"context"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// The catalog and live qualification directory belong to one consistent read.
// This interface deliberately exposes no business or notification writes.
type ReleaseOrderListReader interface {
	releaseApprovalReader
	ReadReleaseHeader(context.Context, string) (domain.ReleaseHeader, error)
	ReadApprovalNotification(context.Context, string, string) (domain.ApprovalNotification, error)
	ListReleaseOrders(context.Context, domain.ReleaseFilter) ([]domain.ReleaseOrderSummary, error)
}

func validateReleaseFilter(filter ReleaseFilter) error {
	if filter.Limit < 1 || filter.Limit > 100 || len(filter.TableName) > 256 || len(filter.ApplicantID) > 36 || len(filter.ID) > 32 || len(filter.After) > 32 {
		return ErrReleaseInvalid
	}
	switch filter.State {
	case "", "DRAFT", "PENDING_PUBLICATION", "PENDING_APPROVAL", "APPROVED", "SUCCEEDED", "COMPLETED", "REJECTED", "CANCELLED", "ROLLED_BACK":
		return nil
	default:
		return ErrReleaseInvalid
	}
}

// NotificationOrders reads submitted orders, filtered before public pagination.
// Approval history and submission facts remain independent of today's eligibility.
func (r *ReleaseOrders) NotificationOrders(ctx context.Context, view string, filter ReleaseFilter) ([]domain.ReleaseOrderSummary, string, error) {
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return nil, "", err
	}
	if err = validateReleaseFilter(filter); err != nil {
		return nil, "", err
	}
	filter.SubmittedOnly = true
	switch view {
	case "pending", "all":
	case "mine":
		if filter.ApplicantID != "" && filter.ApplicantID != actor {
			return nil, "", ErrReleaseInvalid
		}
		filter.ApplicantID = actor
	case "handled":
		filter.ReviewedBy = actor
	default:
		return nil, "", ErrReleaseInvalid
	}
	result := []domain.ReleaseOrderSummary{}
	if view == "pending" {
		if filter.State != "" && filter.State != "PENDING_APPROVAL" {
			return result, "", nil
		}
		filter.State = "PENDING_APPROVAL"
	}
	limit := filter.Limit
	err = r.store.ReadReleaseOrderList(ctx, func(reader ReleaseOrderListReader) error {
		// Scan candidate batches until the visible page and one lookahead are
		// known. The internal cursor advances past every candidate, including
		// whole batches the caller cannot approve; none become empty public pages.
		for {
			orders, err := reader.ListReleaseOrders(ctx, filter)
			if err != nil {
				return err
			}
			for _, summary := range orders {
				order := domain.ReleaseOrder{RollbackTableFlows: summary.RollbackTableFlows, EmergencyReason: summary.EmergencyReason, ReleaseType: summary.ReleaseType, TableFlows: summary.TableFlows, MissingFlowTables: summary.MissingFlowTables, ID: summary.ID, Version: summary.Version, State: summary.State, ApplicantID: summary.ApplicantID, TableNames: summary.TableNames, Approvals: summary.Approvals}
				environment, err := reader.ReadApprovalEnvironment(ctx, order)
				if err != nil {
					return err
				}
				summary.Notification, err = reader.ReadApprovalNotification(ctx, actor, summary.ID)
				if err != nil {
					return err
				}
				if filter.UnreadOnly && !summary.Notification.Unread {
					continue
				}
				summary.Approvals = environment.Approvals
				summary.ApprovalContext, _ = approvalContext(environment, order, actor)
				if view != "pending" || len(summary.ApprovalContext.ApprovableTables) > 0 {
					result = append(result, summary)
					if len(result) > limit {
						return nil
					}
				}
			}
			if len(orders) < filter.Limit {
				return nil
			}
			filter.After = orders[len(orders)-1].ID
		}
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(result) > limit {
		result = result[:limit]
		next = result[limit-1].ID
	}
	return result, next, nil
}
