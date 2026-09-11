package mysql

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

// The order document owns workflow only. Items and actual results are stored
// once per ordered detail, while successful executions carry bounded summaries.
func encodeReleaseHeader(order domain.ReleaseOrder) ([]byte, error) {
	encoded, err := json.Marshal(order)
	if err != nil {
		return nil, err
	}
	var header map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &header); err != nil {
		return nil, err
	}
	for _, key := range []string{"items", "executions", "approval_context"} {
		delete(header, key)
	}
	summary := order.Summary()
	header["item_count"], _ = json.Marshal(summary.ItemCount)
	header["operation_counts"], _ = json.Marshal(summary.OperationCounts)
	return json.Marshal(header)
}

func (s *releaseOrderSession) saveReleaseDetails(ctx context.Context, order domain.ReleaseOrder, previousCount int) error {
	if err := order.VerifyPublication(); err != nil {
		return application.ErrReleaseUnavailable
	}
	// The owning order is locked by the caller; a draft replacement and all its
	// details are one transaction. Submitted intent is supplied unchanged.
	var values []string
	var arguments []any
	encodedBytes := 0
	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_details(order_id,position,table_name,application,publication,rollback) VALUES`+strings.Join(values, ",")+` ON DUPLICATE KEY UPDATE table_name=VALUES(table_name),application=VALUES(application),publication=VALUES(publication),rollback=VALUES(rollback)`, arguments...).Error
		values, arguments, encodedBytes = nil, nil, 0
		return err
	}
	for index, item := range order.Items {
		intent := item
		intent.Publication, intent.Rollback = nil, nil
		applicationJSON, err := json.Marshal(intent)
		if err != nil {
			return application.ErrReleaseUnavailable
		}
		var publication, rollback any
		itemBytes := len(applicationJSON)
		if item.Publication != nil {
			value, e := json.Marshal(item.Publication)
			if e != nil {
				return application.ErrReleaseUnavailable
			}
			publication = value
			itemBytes += len(value)
		}
		if item.Rollback != nil {
			value, e := json.Marshal(item.Rollback)
			if e != nil {
				return application.ErrReleaseUnavailable
			}
			rollback = value
			itemBytes += len(value)
		}
		// This is a statement assembly threshold, never an accepted-data cap.
		if len(values) == 100 || encodedBytes+itemBytes > 1<<20 {
			if err := flush(); err != nil {
				return application.ErrReleaseUnavailable
			}
		}
		encodedBytes += itemBytes
		values = append(values, "(?,?,?,?,?,?)")
		arguments = append(arguments, order.ID, index, item.TableName, applicationJSON, publication, rollback)
	}
	if err := flush(); err != nil {
		return application.ErrReleaseUnavailable
	}
	// Delete only known prior rows. An empty trailing range would lock gaps
	// shared with independent orders that are inserting their own details.
	if previousCount > len(order.Items) {
		positions := releaseDetailPositions(len(order.Items), previousCount)
		if err := s.database.WithContext(ctx).Exec(`DELETE FROM rcc_release_details WHERE order_id=? AND position IN ?`, order.ID, positions).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	for _, execution := range order.Executions {
		encoded, err := json.Marshal(execution)
		if err != nil {
			return application.ErrReleaseUnavailable
		}
		// A workflow update never rewrites an already committed execution.
		if err = s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_executions(order_id,kind,execution_id,document) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE order_id=order_id`, order.ID, execution.Kind, execution.ID, encoded).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	return nil
}

func readReleaseDetails(ctx context.Context, db *gorm.DB, order *domain.ReleaseOrder, loadIntent, lock bool) error {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var details []struct {
		Position                           int
		TableName                          string
		Application, Publication, Rollback []byte
	}
	// The header (or immutable replay snapshot) supplies exact existing keys.
	// No range read is needed for an empty order and no gap lock is acquired.
	if len(order.Items) > 0 {
		positions := releaseDetailPositions(0, len(order.Items))
		if err := db.WithContext(ctx).Raw(`SELECT position,table_name,application,publication,rollback FROM rcc_release_details WHERE order_id=? AND position IN ? ORDER BY position`+suffix, order.ID, positions).Scan(&details).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	if len(details) != len(order.Items) {
		return application.ErrReleaseUnavailable
	}
	if loadIntent {
		order.Items = make([]domain.ReleaseItem, len(details))
		// The locked workflow tells us which immutable executions must exist.
		// Read their complete primary keys: locking an empty order range would
		// take gap locks and deadlock independent first publications on insert.
		var kinds []string
		switch order.State {
		case "SUCCEEDED", "COMPLETED":
			kinds = []string{"PUBLICATION"}
		case "ROLLED_BACK":
			kinds = []string{"PUBLICATION", "ROLLBACK"}
		}
		for _, kind := range kinds {
			var stored struct {
				Kind, ExecutionID string
				Document          []byte
			}
			if err := db.WithContext(ctx).Raw(`SELECT kind,execution_id,document FROM rcc_release_executions WHERE order_id=? AND kind=?`+suffix, order.ID, kind).Row().Scan(&stored.Kind, &stored.ExecutionID, &stored.Document); err != nil {
				return application.ErrReleaseUnavailable
			}
			var execution domain.ReleaseExecution
			if json.Unmarshal(stored.Document, &execution) != nil || execution.Kind != stored.Kind || execution.ID != stored.ExecutionID {
				return application.ErrReleaseUnavailable
			}
			order.Executions = append(order.Executions, execution)

		}
	}
	if len(order.Items) != len(details) {
		return application.ErrReleaseUnavailable
	}
	for index, detail := range details {
		if detail.Position != index {
			return application.ErrReleaseUnavailable
		}
		if loadIntent && json.Unmarshal(detail.Application, &order.Items[index]) != nil {
			return application.ErrReleaseUnavailable
		}
		if detail.TableName != order.Items[index].TableName {
			return application.ErrReleaseUnavailable
		}
		if len(order.Executions) >= 1 && json.Unmarshal(detail.Publication, &order.Items[index].Publication) != nil {
			return application.ErrReleaseUnavailable
		}
		if len(order.Executions) == 2 && json.Unmarshal(detail.Rollback, &order.Items[index].Rollback) != nil {
			return application.ErrReleaseUnavailable
		}
	}
	if order.VerifyPublication() != nil {
		return application.ErrReleaseUnavailable
	}
	return nil
}

func releaseDetailPositions(start, end int) []int {
	positions := make([]int, end-start)
	for index := range positions {
		positions[index] = start + index
	}
	return positions
}

// Idempotency snapshots retain the exact acknowledged workflow and application,
// but reference immutable actual results in details instead of copying them.
func encodeReleaseRequestResult(order domain.ReleaseOrder) ([]byte, error) {
	order.Items = append([]domain.ReleaseItem(nil), order.Items...)
	for index := range order.Items {
		order.Items[index].Publication, order.Items[index].Rollback = nil, nil
	}
	return json.Marshal(order)
}
