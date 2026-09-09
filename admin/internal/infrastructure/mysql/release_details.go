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
	for _, key := range []string{"items", "publication", "rollback", "executions"} {
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
	for index, item := range order.Items {
		applicationJSON, err := json.Marshal(item)
		if err != nil {
			return application.ErrReleaseUnavailable
		}
		var publication, rollback any
		if order.Publication != nil {
			value, e := json.Marshal(order.Publication.Commands[index])
			if e != nil {
				return application.ErrReleaseUnavailable
			}
			publication = value
		}
		if order.Rollback != nil {
			value, e := json.Marshal(order.Rollback.Commands[len(order.Items)-1-index])
			if e != nil {
				return application.ErrReleaseUnavailable
			}
			rollback = value
		}
		values = append(values, "(?,?,?,?,?,?)")
		table := item.TableName
		if table == "" {
			table = order.TableName
		}
		arguments = append(arguments, order.ID, index, table, applicationJSON, publication, rollback)
		if len(values) == 100 || index == len(order.Items)-1 {
			if err = s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_details(order_id,position,table_name,application,publication,rollback) VALUES`+strings.Join(values, ",")+` ON DUPLICATE KEY UPDATE table_name=VALUES(table_name),application=VALUES(application),publication=VALUES(publication),rollback=VALUES(rollback)`, arguments...).Error; err != nil {
				return application.ErrReleaseUnavailable
			}
			values = nil
			arguments = nil
		}
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
			result := &domain.PublicationResult{ExecutionID: execution.ID, Kind: execution.Kind, PublisherID: execution.ActorID, ExecutedAt: execution.ExecutedAt, TableVersion: execution.TableVersions[order.TableName], Notification: execution.Notification}
			switch execution.Kind {
			case "PUBLICATION":
				order.Publication = result
			case "ROLLBACK":
				order.Rollback = result
			default:
				return application.ErrReleaseUnavailable
			}
		}
	}
	if len(order.Items) != len(details) {
		return application.ErrReleaseUnavailable
	}
	if order.Publication != nil {
		order.Publication.Commands = make([]domain.PublicationCommand, len(details))
	}
	if order.Rollback != nil {
		order.Rollback.Commands = make([]domain.PublicationCommand, len(details))
	}
	for index, detail := range details {
		if detail.Position != index || detail.TableName != order.TableName {
			return application.ErrReleaseUnavailable
		}
		if loadIntent && json.Unmarshal(detail.Application, &order.Items[index]) != nil {
			return application.ErrReleaseUnavailable
		}
		if order.Publication != nil && json.Unmarshal(detail.Publication, &order.Publication.Commands[index]) != nil {
			return application.ErrReleaseUnavailable
		}
		if order.Rollback != nil && json.Unmarshal(detail.Rollback, &order.Rollback.Commands[len(details)-1-index]) != nil {
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
	if order.Publication != nil {
		result := *order.Publication
		result.Commands = nil
		order.Publication = &result
	}
	if order.Rollback != nil {
		result := *order.Rollback
		result.Commands = nil
		order.Rollback = &result
	}
	return json.Marshal(order)
}
