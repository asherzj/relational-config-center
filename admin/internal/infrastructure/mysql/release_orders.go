package mysql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	mysqldriver "github.com/go-sql-driver/mysql"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

type releaseOrderSession struct{ *querySnapshotSession }

func (a *Adapter) ExecuteReleaseOrder(ctx context.Context, execute func(application.ReleaseOrderSession) error) error {
	tx := a.gorm.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return application.ErrReleaseUnavailable
	}
	s := &releaseOrderSession{&querySnapshotSession{adapter: a, database: tx}}
	s.active.Store(true)
	defer func() { s.active.Store(false); _ = tx.Rollback().Error }()
	if err := execute(s); err != nil {
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

func (s *releaseOrderSession) ReadRecordBaseline(ctx context.Context, schema domain.TableSchema, id any) (domain.RecordBaseline, error) {
	if err := s.available(); err != nil {
		return domain.RecordBaseline{}, err
	}
	for _, c := range schema.Columns {
		if c.Type == domain.ColumnTypeUnsupported {
			return domain.RecordBaseline{}, application.ErrReleaseSnapshotUnsupported
		}
	}
	name, key, err := recordIdentity(ctx, s.database, schema.Name, id, false)
	if errors.Is(err, sql.ErrNoRows) {
		name, key, err = missingRecordIdentity(ctx, s.database, schema.Name, id)
	}
	if errors.Is(err, application.ErrInvalidMutation) || errors.Is(err, application.ErrIncompatibleTable) || errors.Is(err, application.ErrReleaseSnapshotUnsupported) {
		return domain.RecordBaseline{}, err
	}
	if err != nil {
		return domain.RecordBaseline{}, application.ErrReleaseUnavailable
	}
	rows, err := s.database.WithContext(ctx).Table(schema.Name).Select("*").Where("`id` = ?", id).Rows()
	if err != nil {
		return domain.RecordBaseline{}, application.ErrReleaseUnavailable
	}
	defer rows.Close()
	data, err := scanJSONStringRows(rows, schema.Columns)
	if err != nil {
		return domain.RecordBaseline{}, application.ErrReleaseUnavailable
	}
	var row domain.Row
	if len(data) == 1 {
		row = data[0]
	}
	for _, value := range row {
		if value != nil && !utf8.ValidString(string(*value)) {
			return domain.RecordBaseline{}, application.ErrReleaseSnapshotUnsupported
		}
	}
	version, err := recordVersion(ctx, s.database, name, key)
	if err != nil {
		return domain.RecordBaseline{}, application.ErrReleaseUnavailable
	}
	generatesID := false
	if column, ok := schema.Column("id"); ok && column.AutoIncrement {
		if err := s.database.WithContext(ctx).Raw("SELECT (?=0) AND FIND_IN_SET('NO_AUTO_VALUE_ON_ZERO',@@session.sql_mode)=0", id).Row().Scan(&generatesID); err != nil {
			return domain.RecordBaseline{}, application.ErrReleaseUnavailable
		}
	}
	return domain.RecordBaseline{TableName: name, Row: row, Key: key, Version: strconv.FormatUint(version, 10), GeneratesIDOnInsert: generatesID}, nil
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
	return order, nil
}
func (a *Adapter) GetReleaseOrder(ctx context.Context, id string) (domain.ReleaseOrder, error) {
	return readReleaseOrder(ctx, a.gorm, id, false)
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
	encoded, err := json.Marshal(order)
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	if create {
		err = s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_orders(id,table_name,applicant_id,state,version,document) VALUES(?,?,?,?,?,?)`, order.ID, order.TableName, order.ApplicantID, order.State, order.Version, encoded).Error
	} else {
		err = s.database.WithContext(ctx).Exec(`UPDATE rcc_release_orders SET state=?,version=?,document=? WHERE id=?`, order.State, order.Version, encoded, order.ID).Error
	}
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	return nil
}
func (s *releaseOrderSession) BeginReleaseRequest(ctx context.Context, actor, operation, key string, digest []byte) (*domain.ReleaseOrder, error) {
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
	var order domain.ReleaseOrder
	if json.Unmarshal(result, &order) != nil {
		return nil, application.ErrReleaseUnavailable
	}
	return &order, nil
}
func (s *releaseOrderSession) CompleteReleaseRequest(ctx context.Context, actor, operation, key string, order domain.ReleaseOrder) error {
	if err := s.available(); err != nil {
		return err
	}
	encoded, err := json.Marshal(order)
	if err != nil {
		return application.ErrReleaseUnavailable
	}
	if err = s.database.WithContext(ctx).Exec(`UPDATE rcc_release_requests SET result=? WHERE actor_id=? AND operation=? AND request_key=?`, encoded, actor, operation, key).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	return nil
}
func (a *Adapter) ListReleaseOrders(ctx context.Context, filter domain.ReleaseFilter) ([]domain.ReleaseOrder, error) {
	query := a.gorm.WithContext(ctx).Table("rcc_release_orders").Select("document").Order("id ASC").Limit(filter.Limit)
	for field, value := range map[string]string{"table_name": filter.TableName, "applicant_id": filter.ApplicantID, "state": filter.State, "id": filter.ID} {
		if value != "" {
			query = query.Where(field+" = ?", value)
		}
	}
	if filter.After != "" {
		query = query.Where("id > ?", filter.After)
	}
	rows, err := query.Rows()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	defer rows.Close()
	result := []domain.ReleaseOrder{}
	for rows.Next() {
		var encoded []byte
		var order domain.ReleaseOrder
		if rows.Scan(&encoded) != nil || json.Unmarshal(encoded, &order) != nil {
			return nil, application.ErrReleaseUnavailable
		}
		result = append(result, order)
	}
	if rows.Err() != nil {
		return nil, application.ErrReleaseUnavailable
	}
	return result, nil
}

// Sorted target locks make a whole submitted set atomic and avoid opposite lock
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
		err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_targets(table_name,record_key,order_id) VALUES(?,?,?)`, target.TableName, target.RecordKey, orderID).Error
		var mysqlError *mysqldriver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return application.ErrReleaseTargetConflict
		}
		if err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	return nil
}
func (s *releaseOrderSession) ReleaseTargets(ctx context.Context, orderID string) error {
	if err := s.available(); err != nil {
		return err
	}
	if err := s.database.WithContext(ctx).Exec(`DELETE FROM rcc_release_targets WHERE order_id=?`, orderID).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	return nil
}
