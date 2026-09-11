package mysql

import (
	"context"
	"database/sql"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

type releaseOrderListReader struct{ database *gorm.DB }

func (a *Adapter) ReadReleaseOrderList(ctx context.Context, read func(application.ReleaseOrderListReader) error) error {
	tx := a.gorm.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if tx.Error != nil {
		return application.ErrReleaseUnavailable
	}
	defer tx.Rollback()
	if err := read(&releaseOrderListReader{database: tx}); err != nil {
		return err
	}
	if tx.Commit().Error != nil {
		return application.ErrReleaseUnavailable
	}
	return nil
}

func (r *releaseOrderListReader) ListReleaseOrders(ctx context.Context, filter domain.ReleaseFilter) ([]domain.ReleaseOrderSummary, error) {
	return listReleaseOrders(ctx, r.database, filter)
}

func (r *releaseOrderListReader) ReadApprovalEnvironment(ctx context.Context, order domain.ReleaseOrder) (domain.ReleaseApprovalEnvironment, error) {
	return readApprovalEnvironment(ctx, r.database, order)
}

func (r *releaseOrderListReader) ReadReleaseHeader(ctx context.Context, id string) (domain.ReleaseHeader, error) {
	return readReleaseHeader(ctx, r.database, id)
}
func (r *releaseOrderListReader) ReadApprovalNotification(ctx context.Context, actor, id string) (domain.ApprovalNotification, error) {
	return readApprovalNotification(ctx, r.database, actor, id)
}
