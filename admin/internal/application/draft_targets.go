package application

import (
	"context"
	"errors"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// ReleaseTargetConflict identifies an existing reservation, using the order's
// stable applicant identity. Display names are resolved by the people endpoint.
type ReleaseTargetConflict struct{ TableName, OrderID, ApplicantID string }

func (e *ReleaseTargetConflict) Error() string { return ErrReleaseTargetConflict.Error() }
func (e *ReleaseTargetConflict) Unwrap() error { return ErrReleaseTargetConflict }

func (r *ReleaseOrders) describeTargetConflict(ctx context.Context, err error) error {
	var conflict *ReleaseTargetConflict
	if errors.As(err, &conflict) {
		// The failed save has rolled back. Read the current owner without taking
		// another order's lock inside the target acquisition transaction.
		if owner, readErr := r.store.GetReleaseOrder(ctx, conflict.OrderID); readErr == nil {
			conflict.ApplicantID = owner.ApplicantID
		}
	}
	return err
}

func replaceDraftTargets(ctx context.Context, s ReleaseOrderSession, order ReleaseOrder) error {
	targets := []domain.ActiveTarget{}
	tables := []string{}
	for index, item := range order.Items {
		tables = append(tables, item.TableName)
		for _, key := range item.ConcurrencyKeys {
			targets = append(targets, domain.ActiveTarget{ItemIndex: index, TableName: item.TableName, RecordKey: key})
		}
		if len(item.RecordKey) > 0 {
			targets = append(targets, domain.ActiveTarget{ItemIndex: index, TableName: item.RecordTable, RecordKey: item.RecordKey})
		}
	}
	if err := s.ReplaceReleaseTargets(ctx, order.ID, targets); err != nil {
		return err
	}
	return s.ReplaceReleaseTableReferences(ctx, order.ID, tables)
}
