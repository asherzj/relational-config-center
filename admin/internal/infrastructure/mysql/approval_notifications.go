package mysql

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

const approvalNotificationTable = "rcc_approval_notifications"

type storedApprovalNotification struct {
	AccountID       string
	OrderID         string
	Sequence        uint64
	Pending         bool
	PendingSequence uint64
	ResultSequence  uint64
	ReadSequence    uint64
}

func (row storedApprovalNotification) view() domain.ApprovalNotification {
	return domain.ApprovalNotification{Sequence: strconv.FormatUint(row.Sequence, 10), Pending: row.Pending, Unread: row.PendingSequence > row.ReadSequence || row.ResultSequence > row.ReadSequence}
}

// RecordApprovalNotifications belongs to the enclosing business transaction, after
// its successful state change and before saving the original request result.
func (s *releaseOrderSession) RecordApprovalNotifications(ctx context.Context, order domain.ReleaseOrder, actor string, resultRecipients []string) error {
	if err := s.available(); err != nil {
		return err
	}
	return recordApprovalNotifications(ctx, s.database, order, actor, resultRecipients)
}
func recordApprovalNotifications(ctx context.Context, tx *gorm.DB, order domain.ReleaseOrder, actor string, resultRecipients []string) error {
	recipients := []string{}
	if order.State == "PENDING_APPROVAL" {
		environment, err := readApprovalEnvironment(ctx, tx, order)
		if err != nil {
			return err
		}
		recipients = application.ApprovalRecipients(environment, order)
	}
	var rows []storedApprovalNotification
	if err := tx.WithContext(ctx).Table(approvalNotificationTable).Where("order_id=?", order.ID).Order("account_id").Find(&rows).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	existing := map[string]storedApprovalNotification{}
	ids := append(slices.Clone(recipients), resultRecipients...)
	for _, row := range rows {
		existing[row.AccountID] = row
		ids = append(ids, row.AccountID)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		row, found := existing[id]
		before := row
		row.AccountID = id
		row.OrderID = order.ID
		pending := slices.Contains(recipients, id)
		newPending := pending && !row.Pending && id != actor
		result := slices.Contains(resultRecipients, id) && id != actor
		if !found && !pending && !result {
			continue
		}
		if newPending || result {
			row.Sequence++
			if row.Sequence == 0 {
				return application.ErrReleaseUnavailable
			}
		}
		if newPending {
			row.PendingSequence = row.Sequence
		}
		if result {
			row.ResultSequence = row.Sequence
		}
		row.Pending = pending
		if !pending {
			row.PendingSequence = 0
		}
		if found && row == before {
			continue
		}
		if !found {
			if err := tx.Table(approvalNotificationTable).Create(&row).Error; err != nil {
				return application.ErrReleaseUnavailable
			}
		} else if err := tx.Table(approvalNotificationTable).Where("account_id=? AND order_id=?", id, order.ID).Updates(map[string]any{"sequence": row.Sequence, "pending": row.Pending, "pending_sequence": row.PendingSequence, "result_sequence": row.ResultSequence}).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	return nil
}
func (a *Adapter) ReadApprovalNotificationCounts(ctx context.Context, actor string) (domain.ApprovalNotificationCounts, error) {
	var counts domain.ApprovalNotificationCounts
	err := a.gorm.WithContext(ctx).Raw("SELECT COALESCE(SUM(pending_sequence>read_sequence OR result_sequence>read_sequence),0) AS unread_count,COALESCE(SUM(pending),0) AS pending_count FROM "+approvalNotificationTable+" WHERE account_id=?", actor).Scan(&counts).Error
	if err != nil {
		return counts, application.ErrReleaseUnavailable
	}
	return counts, nil
}

// Only qualification-changing maintenance calls this while holding auth control
// serialization. Ordinary session activity and all GETs remain read-only here.
func reconcilePendingApprovalNotifications(ctx context.Context, tx *gorm.DB, actor string) error {
	var ids []string
	if err := tx.WithContext(ctx).Table("rcc_release_orders").Where("state=?", "PENDING_APPROVAL").Order("id").Pluck("id", &ids).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	for _, id := range ids {
		header, err := readReleaseHeader(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = recordApprovalNotifications(ctx, tx, header.Workflow(), actor, nil); err != nil {
			return err
		}
	}
	return nil
}

func readApprovalNotification(ctx context.Context, tx *gorm.DB, actor, id string) (domain.ApprovalNotification, error) {
	var row storedApprovalNotification
	err := tx.WithContext(ctx).Table(approvalNotificationTable).Where("account_id=? AND order_id=?", actor, id).Take(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return row.view(), application.ErrReleaseUnavailable
	}
	return row.view(), nil
}
func (a *Adapter) AcknowledgeApprovalNotification(ctx context.Context, actor, id, sequence string) (domain.ApprovalNotification, error) {
	var result domain.ApprovalNotification
	err := a.executeReleaseTransaction(ctx, func(s *releaseOrderSession) error {
		if _, err := s.CurrentReleaseAccount(ctx, actor); err != nil {
			return err
		}
		if _, err := readReleaseHeader(ctx, s.database, id); err != nil {
			return err
		}
		current, err := readApprovalNotification(ctx, s.database, actor, id)
		if err != nil {
			return err
		}
		observed, err := strconv.ParseUint(sequence, 10, 64)
		if err != nil {
			return application.ErrReleaseInvalid
		}
		latest, _ := strconv.ParseUint(current.Sequence, 10, 64)
		if observed > latest {
			return application.ErrReleaseInvalid
		}
		if err = s.database.Table(approvalNotificationTable).Where("account_id=? AND order_id=?", actor, id).Update("read_sequence", gorm.Expr("GREATEST(read_sequence,?)", observed)).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
		result, err = readApprovalNotification(ctx, s.database, actor, id)
		return err
	})
	return result, err
}
