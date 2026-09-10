package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

func readReleaseHeader(ctx context.Context, db *gorm.DB, id string) (domain.ReleaseHeader, error) {
	var encoded []byte
	err := db.WithContext(ctx).Raw(`SELECT document FROM rcc_release_orders WHERE id=?`, id).Row().Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReleaseHeader{}, application.ErrReleaseNotFound
	}
	if err != nil {
		return domain.ReleaseHeader{}, application.ErrReleaseUnavailable
	}
	var header domain.ReleaseHeader
	var shape map[string]json.RawMessage
	if json.Unmarshal(encoded, &header) != nil || json.Unmarshal(encoded, &shape) != nil || shape["item_count"] == nil || shape["items"] != nil || shape["publication"] != nil || shape["rollback"] != nil || header.ID != id || header.ItemCount < 0 || header.ItemCount > 1000 {
		return header, application.ErrReleaseUnavailable
	}
	header.Executions = []domain.ReleaseExecution{}
	var stored []struct {
		Kind        string
		ExecutionID string
		Document    []byte
	}
	if err := db.WithContext(ctx).Raw(`SELECT kind,execution_id,document FROM rcc_release_executions WHERE order_id=? ORDER BY kind LIMIT 3`, id).Scan(&stored).Error; err != nil {
		return header, application.ErrReleaseUnavailable
	}
	for _, row := range stored {
		var execution domain.ReleaseExecution
		if json.Unmarshal(row.Document, &execution) != nil || execution.Kind != row.Kind || execution.ID != row.ExecutionID || execution.ItemCount != header.ItemCount {
			return header, application.ErrReleaseUnavailable
		}
		header.Executions = append(header.Executions, execution)
	}
	if header.Workflow().VerifyExecutionSummaries() != nil {
		return header, application.ErrReleaseUnavailable
	}
	return header, nil
}

func (adapter *Adapter) ReadReleaseHeader(ctx context.Context, id string) (domain.ReleaseHeader, error) {
	var header domain.ReleaseHeader
	err := adapter.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		header, err = readReleaseHeader(ctx, tx, id)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return header, err
}

func (adapter *Adapter) ReadReleaseDetailPage(ctx context.Context, id, version string, offset, limit int) (domain.ReleaseDetailPage, error) {
	var page domain.ReleaseDetailPage
	err := adapter.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		header, err := readReleaseHeader(ctx, tx, id)
		if err != nil {
			return err
		}
		if header.Version != version {
			return application.ErrReleaseVersionConflict
		}
		if offset > header.ItemCount {
			return application.ErrReleaseInvalid
		}
		page = domain.ReleaseDetailPage{OrderID: id, Version: version, ItemCount: header.ItemCount, Offset: offset, Items: []domain.ReleaseItem{}}
		var rows []struct {
			Position                           int
			TableName                          string
			Application, Publication, Rollback []byte
		}
		if err := tx.WithContext(ctx).Raw(`SELECT position,table_name,application,publication,rollback FROM rcc_release_details WHERE order_id=? AND position>=? ORDER BY position LIMIT ?`, id, offset, limit).Scan(&rows).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
		expected := min(limit, header.ItemCount-offset)
		if len(rows) != expected {
			return application.ErrReleaseUnavailable
		}
		order := header.Workflow()
		for index, row := range rows {
			var item domain.ReleaseItem
			if row.Position != offset+index || json.Unmarshal(row.Application, &item) != nil || item.TableName != row.TableName || item.DetailID == "" || item.Publication != nil || item.Rollback != nil {
				return application.ErrReleaseUnavailable
			}
			if row.Publication != nil && json.Unmarshal(row.Publication, &item.Publication) != nil {
				return application.ErrReleaseUnavailable
			}
			if row.Rollback != nil && json.Unmarshal(row.Rollback, &item.Rollback) != nil {
				return application.ErrReleaseUnavailable
			}
			if (item.Publication != nil) != (len(header.Executions) >= 1) || (item.Rollback != nil) != (len(header.Executions) == 2) {
				return application.ErrReleaseUnavailable
			}
			for _, execution := range header.Executions {
				if order.VerifyExecutionDetail(execution, item) != nil {
					return application.ErrReleaseUnavailable
				}
			}
			page.Items = append(page.Items, item)
		}
		next := offset + len(rows)
		if next < header.ItemCount {
			page.NextOffset = &next
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return page, err
}
