package mysql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

type releaseOrderSession struct{ *querySnapshotSession }

func (a *Adapter) ExecuteReleaseOrder(ctx context.Context, execute func(application.ReleaseOrderSession) error) error {
	return a.executeReleaseTransaction(ctx, func(s *releaseOrderSession) error { return execute(s) })
}

func (a *Adapter) ExecutePublication(ctx context.Context, execute func(application.PublicationSession) error) error {
	return a.executeReleaseTransaction(ctx, func(s *releaseOrderSession) error { return execute(&publicationSession{s}) })
}

func (a *Adapter) executeReleaseTransaction(ctx context.Context, execute func(*releaseOrderSession) error) error {
	tx := a.gorm.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return application.ErrReleaseUnavailable
	}
	// Every release write takes authorization before request, order and details.
	var authLock int
	if err := tx.Raw(`SELECT id FROM rcc_auth_control_lock WHERE id=1 FOR UPDATE`).Row().Scan(&authLock); err != nil {
		_ = tx.Rollback().Error
		return application.ErrReleaseUnavailable
	}
	s := &releaseOrderSession{&querySnapshotSession{adapter: a, database: tx}}
	s.active.Store(true)
	defer func() { s.active.Store(false); _ = tx.Rollback().Error }()
	if err := execute(s); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return application.ErrMutationTimeout
		}
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return application.ErrReleaseUnknown
	}
	return nil
}

func (s *releaseOrderSession) DatabaseTime(ctx context.Context) (time.Time, error) {
	if err := s.available(); err != nil {
		return time.Time{}, err
	}
	var now time.Time
	err := s.database.WithContext(ctx).Raw("SELECT UTC_TIMESTAMP(6)").Row().Scan(&now)
	if err != nil {
		return now, application.ErrReleaseUnavailable
	}
	return now, nil
}

func readReleaseOrder(ctx context.Context, db *gorm.DB, id string, lock bool) (domain.ReleaseOrder, error) {
	query := "SELECT document FROM rcc_release_orders WHERE id=?"
	if lock {
		query += " FOR UPDATE"
	}
	var encoded []byte
	err := db.WithContext(ctx).Raw(query, id).Row().Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReleaseOrder{}, application.ErrReleaseNotFound
	}
	if err != nil {
		return domain.ReleaseOrder{}, application.ErrReleaseUnavailable
	}
	var order domain.ReleaseOrder
	if json.Unmarshal(encoded, &order) != nil {
		return order, application.ErrReleaseUnavailable
	}
	// A legacy aggregate document is deliberately unsupported, not migrated.
	var shape map[string]json.RawMessage
	if json.Unmarshal(encoded, &shape) != nil || shape["item_count"] == nil || shape["items"] != nil || shape["publication"] != nil || shape["rollback"] != nil {
		return domain.ReleaseOrder{}, application.ErrReleaseUnavailable
	}
	var summary domain.ReleaseOrderSummary
	if json.Unmarshal(encoded, &summary) != nil || summary.ItemCount < 0 || summary.ItemCount > 1000 {
		return domain.ReleaseOrder{}, application.ErrReleaseUnavailable
	}
	order.Items = make([]domain.ReleaseItem, summary.ItemCount)
	if err := readReleaseDetails(ctx, db, &order, true, lock); err != nil {
		return domain.ReleaseOrder{}, err
	}
	if len(order.Items) != summary.ItemCount {
		return domain.ReleaseOrder{}, application.ErrReleaseUnavailable
	}
	return order, nil
}
func (s *releaseOrderSession) GetReleaseOrder(ctx context.Context, id string) (domain.ReleaseOrder, error) {
	if err := s.available(); err != nil {
		return domain.ReleaseOrder{}, err
	}
	return readReleaseOrder(ctx, s.database, id, true)
}
func (s *releaseOrderSession) SaveReleaseOrder(ctx context.Context, order domain.ReleaseOrder, create bool) error {
	if err := s.available(); err != nil {
		return err
	}
	encoded, err := encodeReleaseHeader(order)
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	previousCount := 0
	if !create {
		if err = s.database.WithContext(ctx).Raw(`SELECT JSON_EXTRACT(document,'$.item_count') FROM rcc_release_orders WHERE id=? FOR UPDATE`, order.ID).Row().Scan(&previousCount); err != nil || previousCount < 0 || previousCount > 1000 {
			return application.ErrReleaseUnavailable
		}
	}
	if create {
		err = s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_orders(id,applicant_id,state,version,document) VALUES(?,?,?,?,?)`, order.ID, order.ApplicantID, order.State, order.Version, encoded).Error
	} else {
		err = s.database.WithContext(ctx).Exec(`UPDATE rcc_release_orders SET state=?,version=?,document=? WHERE id=?`, order.State, order.Version, encoded, order.ID).Error
	}
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	return s.saveReleaseDetails(ctx, order, previousCount)
}
func (s *releaseOrderSession) BeginReleaseRequest(ctx context.Context, actor, operation, key string, digest []byte) (*domain.ReleaseOrder, error) {
	result, err := s.beginReleaseRequestValue(ctx, actor, operation, key, digest)
	if err != nil || result == nil {
		return nil, err
	}
	var order domain.ReleaseOrder
	if json.Unmarshal(result, &order) != nil {
		return nil, application.ErrReleaseUnavailable
	}
	for _, item := range order.Items {
		if item.Publication != nil || item.Rollback != nil {
			return nil, application.ErrReleaseUnavailable
		}
	}
	if len(order.Executions) > 0 {
		// Match every ordinary write's request → order → details lock order.
		var locked string
		if err := s.database.WithContext(ctx).Raw(`SELECT id FROM rcc_release_orders WHERE id=? FOR UPDATE`, order.ID).Row().Scan(&locked); err != nil {
			return nil, application.ErrReleaseUnavailable
		}
		if err := readReleaseDetails(ctx, s.database, &order, false, true); err != nil {
			return nil, err
		}
	} else if order.VerifyPublication() != nil {
		return nil, application.ErrReleaseUnavailable
	}
	return &order, nil
}
func (s *releaseOrderSession) beginReleaseRequestValue(ctx context.Context, actor, operation, key string, digest []byte) ([]byte, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_requests(actor_id,operation,request_key,digest,result) VALUES(?,?,?,?,NULL) ON DUPLICATE KEY UPDATE request_key=request_key`, actor, operation, key, digest).Error; err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	var storedDigest, result []byte
	if err := s.database.WithContext(ctx).Raw(`SELECT digest,result FROM rcc_release_requests WHERE actor_id=? AND operation=? AND request_key=? FOR UPDATE`, actor, operation, key).Row().Scan(&storedDigest, &result); err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	if !bytes.Equal(storedDigest, digest) {
		return nil, application.ErrReleaseIdempotencyConflict
	}
	if result == nil {
		return nil, nil
	}
	return result, nil
}

func (s *releaseOrderSession) BeginRollbackPreviewRequest(ctx context.Context, actor, operation, key string, digest []byte) (*domain.QuickRollbackPreview, error) {
	result, err := s.beginReleaseRequestValue(ctx, actor, operation, key, digest)
	if err != nil || result == nil {
		return nil, err
	}
	var preview domain.QuickRollbackPreview
	if json.Unmarshal(result, &preview) != nil || preview.OrderID == "" || len(preview.TableFlows) == 0 {
		return nil, application.ErrReleaseUnavailable
	}
	return &preview, nil
}

func (s *releaseOrderSession) CompleteRollbackPreviewRequest(ctx context.Context, actor, operation, key string, preview domain.QuickRollbackPreview) error {
	if err := s.available(); err != nil {
		return err
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	if err = s.database.WithContext(ctx).Exec(`UPDATE rcc_release_requests SET result=? WHERE actor_id=? AND operation=? AND request_key=?`, encoded, actor, operation, key).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	return nil
}

func (s *releaseOrderSession) CompleteReleaseRequest(ctx context.Context, actor, operation, key string, order domain.ReleaseOrder) error {
	if err := s.available(); err != nil {
		return err
	}
	encoded, err := encodeReleaseRequestResult(order)
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	if err = s.database.WithContext(ctx).Exec(`UPDATE rcc_release_requests SET result=? WHERE actor_id=? AND operation=? AND request_key=?`, encoded, actor, operation, key).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	return nil
}
func (a *Adapter) ListReleaseOrders(ctx context.Context, filter domain.ReleaseFilter) ([]domain.ReleaseOrderSummary, error) {
	return listReleaseOrders(ctx, a.gorm, filter)
}
func listReleaseOrders(ctx context.Context, database *gorm.DB, filter domain.ReleaseFilter) ([]domain.ReleaseOrderSummary, error) {
	query := database.WithContext(ctx).Table("rcc_release_orders").Select("document").Order("id ASC").Limit(filter.Limit)
	if filter.SubmittedOnly {
		query = query.Where(`JSON_CONTAINS(document, '{"action":"SUBMIT"}', '$.history')`)
	}
	if filter.ReviewedBy != "" {
		approve, _ := json.Marshal(map[string]string{"action": "APPROVE", "actor_id": filter.ReviewedBy})
		reject, _ := json.Marshal(map[string]string{"action": "REJECT", "actor_id": filter.ReviewedBy})
		query = query.Where(`(JSON_CONTAINS(document, ?, '$.history') OR JSON_CONTAINS(document, ?, '$.history'))`, string(approve), string(reject))
	}
	for field, value := range map[string]string{"applicant_id": filter.ApplicantID, "state": filter.State, "id": filter.ID} {
		if value != "" {
			query = query.Where(field+" = ?", value)
		}
	}
	if filter.TableName != "" {
		query = query.Where("EXISTS (SELECT 1 FROM rcc_release_details d WHERE d.order_id=rcc_release_orders.id AND d.table_name=?)", filter.TableName)
	}
	if filter.After != "" {
		query = query.Where("id > ?", filter.After)
	}
	rows, err := query.Rows()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	defer rows.Close()
	result := []domain.ReleaseOrderSummary{}
	for rows.Next() {
		var encoded []byte
		if rows.Scan(&encoded) != nil {
			return nil, application.ErrReleaseUnavailable
		}
		var summary domain.ReleaseOrderSummary
		if json.Unmarshal(encoded, &summary) != nil {
			return nil, application.ErrReleaseUnavailable
		}
		result = append(result, summary)
	}
	if rows.Err() != nil {
		return nil, application.ErrReleaseUnavailable
	}
	return result, nil
}

// Sorted target locks make a whole saved set atomic and avoid opposite lock
// order for overlapping sets. Unknown auto-increment ids contribute no target.
func (s *releaseOrderSession) ReserveReleaseTargets(ctx context.Context, orderID string, targets []domain.ActiveTarget) error {
	if err := s.available(); err != nil {
		return err
	}
	targets = append([]domain.ActiveTarget(nil), targets...)
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].TableName != targets[j].TableName {
			return targets[i].TableName < targets[j].TableName
		}
		return bytes.Compare(targets[i].RecordKey, targets[j].RecordKey) < 0
	})
	for _, target := range targets {
		err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_targets(table_name,record_key,order_id) VALUES(?,?,?) ON DUPLICATE KEY UPDATE order_id=order_id`, target.TableName, target.RecordKey, orderID).Error
		if err != nil {
			return application.ErrReleaseUnavailable
		}
		var owner string
		if err := s.database.WithContext(ctx).Raw(`SELECT order_id FROM rcc_release_targets WHERE table_name=? AND record_key=? FOR UPDATE`, target.TableName, target.RecordKey).Row().Scan(&owner); err != nil {
			return application.ErrReleaseUnavailable
		}
		if owner != orderID {
			return &application.ReleaseItemError{Index: target.ItemIndex, Cause: &application.ReleaseTargetConflict{TableName: target.TableName, OrderID: owner}}
		}
	}
	return nil
}
func (s *releaseOrderSession) ReleaseTargets(ctx context.Context, orderID string) error {
	if err := s.available(); err != nil {
		return err
	}
	if err := s.ReplaceReleaseTargets(ctx, orderID, nil); err != nil {
		return err
	}
	return s.ReplaceReleaseTableReferences(ctx, orderID, nil)
}

func decodeStoredReleaseOrder(encoded []byte) (domain.ReleaseOrder, error) {
	var order domain.ReleaseOrder
	if json.Unmarshal(encoded, &order) != nil || order.VerifyPublication() != nil {
		return domain.ReleaseOrder{}, application.ErrReleaseUnavailable
	}
	return order, nil
}
func (a *Adapter) AccountDisplayNames(ctx context.Context, ids []string) (map[string]string, error) {
	result := map[string]string{}
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		var people []struct{ ID, DisplayName string }
		if err := a.gorm.WithContext(ctx).Table(accountTable).Select("id,display_name").Where("id IN ?", ids[start:end]).Scan(&people).Error; err != nil {
			return nil, application.ErrReleaseUnavailable
		}
		for _, person := range people {
			result[person.ID] = person.DisplayName
		}
	}
	return result, nil
}

// AppendReleaseFailure locks only the current main record and appends audit.
// It never rewrites detail facts, advances the business CAS, or saves a stale
// pre-execution aggregate over a concurrent legitimate workflow change.
func (s *releaseOrderSession) AppendReleaseFailure(ctx context.Context, id string, event domain.ReleaseEvent) error {
	if err := s.available(); err != nil {
		return err
	}
	var locked string
	if err := s.database.WithContext(ctx).Raw("SELECT id FROM rcc_release_orders WHERE id=? FOR UPDATE", id).Row().Scan(&locked); err != nil {
		return application.ErrReleaseUnavailable
	}
	now, err := s.DatabaseTime(ctx)
	if err != nil {
		return err
	}
	event.At = now.UTC().Format(time.RFC3339Nano)
	return s.appendReleaseEvent(ctx, id, event)
}

// AppendRollbackReason changes only append-only audit state. In particular it
// does not rewrite detail rows, successful executions, workflow columns or CAS.
func (s *releaseOrderSession) AppendRollbackReason(ctx context.Context, id string, event domain.ReleaseEvent) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.appendReleaseEvent(ctx, id, event)
}

func (s *releaseOrderSession) appendReleaseEvent(ctx context.Context, id string, event domain.ReleaseEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	result := s.database.WithContext(ctx).Exec("UPDATE rcc_release_orders SET document=JSON_ARRAY_APPEND(document,'$.history',CAST(? AS JSON)) WHERE id=?", string(encoded), id)
	if result.Error != nil {
		return application.ErrReleaseUnavailable
	}
	if result.RowsAffected != 1 {
		return application.ErrReleaseNotFound
	}
	return nil
}
