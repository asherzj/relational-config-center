package mysql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

type publicationSession struct{ *releaseOrderSession }

func (s *publicationSession) LockPublicationTable(ctx context.Context, table string) error {
	if err := s.available(); err != nil {
		return err
	}
	// An update lock on this table's progress orders commits and cursors. Tables
	// are independent; neither a global auto-increment nor an external call orders them.
	if err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_table_publications(table_name,table_version,command_cursor) VALUES(?,0,0) ON DUPLICATE KEY UPDATE table_name=table_name`, table).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	// Lock current catalog definitions before the first consistent snapshot read.
	var queryCode, mutationCode string
	if err := s.database.WithContext(ctx).Raw(`SELECT query_policy_code,mutation_policy_code FROM rcc_table_policies WHERE table_name=? FOR SHARE`, table).Row().Scan(&queryCode, &mutationCode); err != nil {
		return application.ErrReleaseUnavailable
	}
	for _, policy := range []struct{ table, code string }{{"rcc_query_policies", queryCode}, {"rcc_mutation_policies", mutationCode}} {
		var code string
		if err := s.database.WithContext(ctx).Raw("SELECT code FROM "+policy.table+" WHERE code=? FOR SHARE", policy.code).Row().Scan(&code); err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	return nil
}
func (s *publicationSession) CommitPublication(ctx context.Context, plan application.PublicationPlan) (domain.PublicationResult, error) {
	if err := s.available(); err != nil {
		return domain.PublicationResult{}, err
	}
	if err := s.checkPublicationCapability(ctx, plan); err != nil {
		return domain.PublicationResult{}, err
	}
	var version, cursor uint64
	if err := s.database.WithContext(ctx).Raw(`SELECT table_version,command_cursor FROM rcc_table_publications WHERE table_name=? FOR UPDATE`, plan.Execution.TableName).Row().Scan(&version, &cursor); err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	if version == math.MaxUint64 || uint64(len(plan.Items)) > math.MaxUint64-cursor {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	version++
	result := domain.PublicationResult{TableVersion: strconv.FormatUint(version, 10), PublisherID: plan.PublisherID, ExecutedAt: plan.At.UTC().Format(time.RFC3339Nano), Commands: []domain.PublicationCommand{}, Notification: domain.RefreshNotification{ID: plan.OrderID, TableVersion: strconv.FormatUint(version, 10), Status: "NOT_CONNECTED"}}
	for _, item := range plan.Items {
		before, err := s.readCanonicalRow(ctx, plan, item.ID, true)
		if err != nil {
			return domain.PublicationResult{}, err
		}
		if item.Intent.Operation == "ADD" && !before.Deleted {
			return domain.PublicationResult{}, application.ErrDuplicateKey
		}
		if item.Intent.Operation != "ADD" && before.Deleted {
			return domain.PublicationResult{}, application.ErrMutationRowNotFound
		}
		id := item.ID
		// Lock/version the existing record before update/delete. ADD checks the
		// tombstone/floor after the actual insert, still inside this transaction.
		if item.Intent.Operation != "ADD" {
			if err = compareAndAdvanceRecordVersion(ctx, s.database, plan.Schema.Name, id, item.Intent.ExpectedRecordVersion); err != nil {
				return domain.PublicationResult{}, err
			}
		}
		values := map[string]any{}
		for _, value := range item.Values {
			values[value.Column.Name] = value.Value
		}
		db := s.database.WithContext(ctx).Session(&gorm.Session{SkipDefaultTransaction: true})
		var changed *gorm.DB
		switch item.Intent.Operation {
		case "ADD":
			if len(values) == 0 {
				changed = db.Exec("INSERT INTO " + db.Statement.Quote(plan.Schema.Name) + " VALUES ()")
			} else {
				changed = db.Table(plan.Schema.Name).Create(values)
			}
		case "MODIFY":
			changed = db.Table(plan.Schema.Name).Where("`id`=?", id).Updates(values)
		case "DELETE":
			changed = db.Table(plan.Schema.Name).Where("`id`=?", id).Delete(&map[string]any{})
		default:
			return domain.PublicationResult{}, application.ErrInvalidMutation
		}
		if changed.Error != nil {
			return domain.PublicationResult{}, classifyMutationError(changed.Error, ctx.Err())
		}
		if changed.RowsAffected != 1 {
			return domain.PublicationResult{}, application.ErrMutationRowNotFound
		}
		if item.Intent.Operation == "ADD" {
			if id == nil {
				var actual string
				if err := db.Raw("SELECT CAST(LAST_INSERT_ID() AS CHAR)").Row().Scan(&actual); err != nil {
					return domain.PublicationResult{}, application.ErrReleaseUnavailable
				}
				id = actual
			}
			name, key, err := recordIdentity(ctx, s.database, plan.Schema.Name, id, true)
			if err != nil {
				return domain.PublicationResult{}, classifyMutationError(err, ctx.Err())
			}
			var owner string
			err = s.database.WithContext(ctx).Raw(`SELECT order_id FROM rcc_release_targets WHERE table_name=? AND record_key=? FOR UPDATE`, name, key).Row().Scan(&owner)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return domain.PublicationResult{}, application.ErrReleaseUnavailable
			}
			if err == nil && owner != plan.OrderID {
				return domain.PublicationResult{}, application.ErrReleaseTargetConflict
			}
			var expected *string
			if item.Intent.ID != nil {
				if !bytes.Equal(item.Intent.RecordKey, key) || item.Intent.RecordTable != name {
					return domain.PublicationResult{}, application.ErrRecordVersionConflict
				}
				expected = &item.Intent.ExpectedRecordVersion
			}
			if err = advanceRecordVersion(ctx, s.database, plan.Schema.Name, id, expected); err != nil {
				return domain.PublicationResult{}, err
			}
		}
		name, key, err := recordIdentity(ctx, s.database, plan.Schema.Name, id, true)
		if item.Intent.Operation == "DELETE" {
			name = item.Intent.RecordTable
			key = item.Intent.RecordKey
			err = nil
		}
		if err != nil {
			return domain.PublicationResult{}, application.ErrReleaseUnavailable
		}
		recordVersion, err := recordVersion(ctx, s.database, name, key)
		if err != nil {
			return domain.PublicationResult{}, application.ErrReleaseUnavailable
		}
		final, err := s.readCanonicalRow(ctx, plan, id, true)
		if err != nil {
			return domain.PublicationResult{}, err
		}
		identityRow := final
		if final.Deleted {
			identityRow = before
		}
		actualID, err := identityRow.RecordID()
		if err != nil || final.Deleted != (item.Intent.Operation == "DELETE") {
			return domain.PublicationResult{}, application.ErrPublicationUnsupported
		}
		cursor++
		command := domain.PublicationCommand{OrderID: plan.OrderID, Sequence: strconv.FormatUint(cursor, 10), TableName: plan.Execution.TableName, TableVersion: result.TableVersion, Operation: item.Intent.Operation, ID: actualID, RecordVersion: strconv.FormatUint(recordVersion, 10), Before: before, Final: final}
		encoded, err := json.Marshal(command)
		if err != nil {
			return domain.PublicationResult{}, application.ErrReleaseUnavailable
		}
		if err = s.database.WithContext(ctx).Exec(`INSERT INTO rcc_publication_commands(table_name,sequence,order_id,document) VALUES(?,?,?,?)`, command.TableName, command.Sequence, plan.OrderID, encoded).Error; err != nil {
			return domain.PublicationResult{}, application.ErrReleaseUnavailable
		}
		result.Commands = append(result.Commands, command)
	}
	if err := s.database.WithContext(ctx).Exec(`UPDATE rcc_table_publications SET table_version=?,command_cursor=? WHERE table_name=?`, version, cursor, plan.Execution.TableName).Error; err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	encoded, err := json.Marshal(result.Notification)
	if err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	if err = s.database.WithContext(ctx).Exec(`INSERT INTO rcc_refresh_notifications(order_id,table_name,table_version,document) VALUES(?,?,?,?)`, plan.OrderID, plan.Execution.TableName, version, encoded).Error; err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	return result, nil
}

func (s *publicationSession) readCanonicalRow(ctx context.Context, plan application.PublicationPlan, id any, lock bool) (domain.CanonicalRow, error) {
	if id == nil {
		return domain.NewCanonicalRow(plan.SchemaDigest, true, nil)
	}
	// CAST keeps zero dates, signed TIME durations and unsigned BIGINT out of
	// driver coercion. Text is explicitly transcoded by MySQL to UTF-8.
	projections := []string{}
	types := map[string]string{}
	for _, section := range plan.Execution.Sections {
		if section.Name == "columns" {
			for _, row := range section.Rows {
				if len(row) != 10 || row[0] == nil || row[2] == nil {
					return domain.CanonicalRow{}, application.ErrPublicationUnsupported
				}
				types[*row[0]] = *row[2]
			}
		}
	}
	for _, column := range plan.Schema.Columns {
		expression := s.database.Statement.Quote(column.Name)
		if column.Type == domain.ColumnTypeFloat64 {
			expression = "CAST(" + expression + " AS DOUBLE)"
		}
		projections = append(projections, "CAST("+expression+" AS CHAR CHARACTER SET utf8mb4)")
	}
	query := "SELECT " + strings.Join(projections, ",") + " FROM " + s.database.Statement.Quote(plan.Schema.Name) + " WHERE `id`=?"
	if lock {
		query += " FOR UPDATE"
	}
	values := make([]*string, len(plan.Schema.Columns))
	dest := make([]any, len(values))
	for i := range values {
		dest[i] = &values[i]
	}
	err := s.database.WithContext(ctx).Raw(query, id).Row().Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewCanonicalRow(plan.SchemaDigest, true, nil)
	}
	if err != nil {
		return domain.CanonicalRow{}, classifyMutationError(err, ctx.Err())
	}
	fields := []domain.CanonicalField{}
	for i, column := range plan.Schema.Columns {
		encoding := "text"
		if column.Type == domain.ColumnTypeJSON {
			encoding = "json"
		}
		if values[i] == nil {
			encoding = "sql_null"
		} else if !utf8.ValidString(*values[i]) {
			return domain.CanonicalRow{}, application.ErrPublicationUnsupported
		}
		fields = append(fields, domain.CanonicalField{Name: column.Name, Type: types[column.Name], Encoding: encoding, Value: values[i]})
	}
	row, err := domain.NewCanonicalRow(plan.SchemaDigest, false, fields)
	if err != nil {
		return row, application.ErrPublicationUnsupported
	}
	return row, nil
}

var _ application.PublicationSession = (*publicationSession)(nil)
var _ application.ReleaseOrderStore = (*Adapter)(nil)
