package mysql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	executionID := plan.OrderID + ":" + plan.ExecutionKind
	result := domain.PublicationResult{ExecutionID: executionID, Kind: plan.ExecutionKind, TableVersion: strconv.FormatUint(version, 10), PublisherID: plan.PublisherID, ExecutedAt: plan.At.UTC().Format(time.RFC3339Nano), Commands: []domain.PublicationCommand{}, Notification: domain.RefreshNotification{ID: executionID, TableVersion: strconv.FormatUint(version, 10), Status: "NOT_CONNECTED"}}
	ids := make([]any, len(plan.Items))
	for i, item := range plan.Items {
		ids[i] = item.ID
	}
	beforeRows, err := s.readCanonicalRows(ctx, plan, ids)
	if err != nil {
		return domain.PublicationResult{}, err
	}
	for index, item := range plan.Items {
		before := beforeRows[index]
		if item.Intent.Operation == "ADD" && !before.Deleted {
			return domain.PublicationResult{}, &application.ReleaseItemError{Index: index, Cause: application.ErrDuplicateKey}
		}
		if item.Intent.Operation != "ADD" && before.Deleted {
			return domain.PublicationResult{}, &application.ReleaseItemError{Index: index, Cause: application.ErrMutationRowNotFound}
		}
		values := map[string]any{}
		for _, value := range item.Values {
			values[value.Column.Name] = value.Value
		}
		insertResult := gorm.WithResult()
		db := s.database.WithContext(ctx).Session(&gorm.Session{SkipDefaultTransaction: true}).Clauses(insertResult)
		var changed *gorm.DB
		switch item.Intent.Operation {
		case "ADD":
			if len(values) == 0 {
				changed = db.Exec("INSERT INTO " + db.Statement.Quote(plan.Schema.Name) + " VALUES ()")
			} else {
				changed = db.Table(plan.Schema.Name).Create(values)
			}
		case "MODIFY":
			changed = db.Table(plan.Schema.Name).Where("`id`=?", ids[index]).Updates(values)
		case "DELETE":
			changed = db.Table(plan.Schema.Name).Where("`id`=?", ids[index]).Delete(&map[string]any{})
		default:
			return domain.PublicationResult{}, application.ErrInvalidMutation
		}
		if changed.Error != nil {
			return domain.PublicationResult{}, &application.ReleaseItemError{Index: index, Cause: classifyMutationError(changed.Error, ctx.Err())}
		}
		if changed.RowsAffected != 1 {
			return domain.PublicationResult{}, &application.ReleaseItemError{Index: index, Cause: application.ErrMutationRowNotFound}
		}
		if item.Intent.Operation == "ADD" && ids[index] == nil {
			// Use this INSERT's protocol result, not mutable connection state.
			// MySQL exposes uint64 insert ids through database/sql's int64 method.
			if insertResult.Result == nil {
				return domain.PublicationResult{}, application.ErrReleaseUnavailable
			}
			actual, err := insertResult.Result.LastInsertId()
			if err != nil {
				return domain.PublicationResult{}, application.ErrReleaseUnavailable
			}
			ids[index] = strconv.FormatUint(uint64(actual), 10)
		}
	}
	finalRows, err := s.readCanonicalRows(ctx, plan, ids)
	if err != nil {
		return domain.PublicationResult{}, err
	}
	// A deleted row keeps its locked pre-publication identity as a tombstone.
	// Read actual post-write identities only for rows that still exist.
	identityIDs := append([]any(nil), ids...)
	for index, item := range plan.Items {
		if item.Intent.Operation == "DELETE" {
			identityIDs[index] = nil
		}
	}
	baselines, err := s.ReadRecordBaselines(ctx, plan.Schema, identityIDs)
	if err != nil {
		return domain.PublicationResult{}, err
	}
	for index, item := range plan.Items {
		if item.Intent.Operation == "DELETE" {
			baselines[index] = domain.RecordBaseline{TableName: item.Intent.RecordTable, Key: item.Intent.RecordKey, Version: item.Intent.ExpectedRecordVersion}
		}
		if item.Intent.ID != nil && (!bytes.Equal(item.Intent.RecordKey, baselines[index].Key) || item.Intent.RecordTable != baselines[index].TableName) {
			return domain.PublicationResult{}, &application.ReleaseItemError{Index: index, Cause: application.ErrRecordVersionConflict}
		}
	}
	// Generated INSERT identities were unknown at submission. Protect the real
	// MySQL identity in this same transaction before acknowledging publication.
	generatedTargets := []domain.ActiveTarget{}
	for index, item := range plan.Items {
		if item.Intent.ID == nil {
			baseline := baselines[index]
			generatedTargets = append(generatedTargets, domain.ActiveTarget{ItemIndex: index, TableName: baseline.TableName, RecordKey: baseline.Key})
		}
	}
	if err := s.ReserveReleaseTargets(ctx, plan.OrderID, generatedTargets); err != nil {
		return domain.PublicationResult{}, err
	}
	versions, err := s.advancePublicationVersions(ctx, plan, baselines)
	if err != nil {
		return domain.PublicationResult{}, err
	}
	placeholders := []string{}
	commandBytes := 0
	arguments := []any{}
	for index, item := range plan.Items {
		before, final := beforeRows[index], finalRows[index]
		identityRow := final
		if final.Deleted {
			identityRow = before
		}
		actualID, err := identityRow.RecordID()
		if err != nil || final.Deleted != (item.Intent.Operation == "DELETE") {
			return domain.PublicationResult{}, &application.ReleaseItemError{Index: index, Cause: application.ErrPublicationUnsupported}
		}
		idColumn, found := plan.Schema.Column("id")
		if !found {
			return domain.PublicationResult{}, application.ErrPublicationUnsupported
		}
		if _, err := domain.ParseColumnValue(idColumn, domain.JSONString(actualID)); err != nil {
			return domain.PublicationResult{}, &application.ReleaseItemError{Index: index, Cause: application.ErrInvalidMutation}
		}
		cursor++
		command := domain.PublicationCommand{ExecutionID: executionID, ExecutionKind: plan.ExecutionKind, OrderID: plan.OrderID, Sequence: strconv.FormatUint(cursor, 10), TableName: plan.Execution.TableName, TableVersion: result.TableVersion, Operation: item.Intent.Operation, ID: actualID, RecordVersion: versions[index], Before: before, Final: final}
		encoded, err := json.Marshal(command)
		if err != nil {
			return domain.PublicationResult{}, application.ErrReleaseUnavailable
		}
		commandBytes += len(encoded)
		if commandBytes > application.ReleaseResultBytes {
			return domain.PublicationResult{}, application.ErrReleaseResultLimit
		}
		placeholders = append(placeholders, "(?,?,?,?,?)")
		arguments = append(arguments, command.TableName, command.Sequence, plan.OrderID, executionID, encoded)
		result.Commands = append(result.Commands, command)
	}
	if err := s.database.WithContext(ctx).Exec("INSERT INTO rcc_publication_commands(table_name,sequence,order_id,execution_id,document) VALUES"+strings.Join(placeholders, ","), arguments...).Error; err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	if err := s.database.WithContext(ctx).Exec(`UPDATE rcc_table_publications SET table_version=?,command_cursor=? WHERE table_name=?`, version, cursor, plan.Execution.TableName).Error; err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	encoded, err := json.Marshal(result.Notification)
	if err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	if err = s.database.WithContext(ctx).Exec(`INSERT INTO rcc_refresh_notifications(order_id,execution_id,table_name,table_version,document) VALUES(?,?,?,?,?)`, plan.OrderID, executionID, plan.Execution.TableName, version, encoded).Error; err != nil {
		return domain.PublicationResult{}, application.ErrReleaseUnavailable
	}
	return result, nil
}

func (s *publicationSession) readCanonicalRows(ctx context.Context, plan application.PublicationPlan, ids []any) ([]domain.CanonicalRow, error) {
	result := make([]domain.CanonicalRow, len(ids))
	knownID := false
	for i, id := range ids {
		result[i], _ = domain.NewCanonicalRow(plan.SchemaDigest, true, nil)
		knownID = knownID || id != nil
	}
	if !knownID {
		return result, nil
	}
	meta, err := recordIdentityMetadata(ctx, s.database, plan.Schema.Name)
	if err != nil {
		return nil, err
	}
	lookup, err := buildRecordLookup(meta, ids)
	if err != nil {
		return nil, err
	}
	// CAST keeps zero dates, signed TIME durations and unsigned BIGINT out of
	// driver coercion. Text is explicitly transcoded by MySQL to UTF-8.
	projections := []string{"requested.ordinal"}
	types := map[string]string{}
	for _, section := range plan.Execution.Sections {
		if section.Name == "columns" {
			for _, row := range section.Rows {
				if len(row) != 10 || row[0] == nil || row[2] == nil {
					return nil, application.ErrPublicationUnsupported
				}
				types[*row[0]] = *row[2]
			}
		}
	}
	for _, column := range plan.Schema.Columns {
		expression := "b." + s.database.Statement.Quote(column.Name)
		if column.Type == domain.ColumnTypeFloat64 {
			expression = "CAST(" + expression + " AS DOUBLE)"
		}
		projections = append(projections, "CAST("+expression+" AS CHAR CHARACTER SET utf8mb4)")
	}
	query := "SELECT " + strings.Join(projections, ",") + " FROM " + lookup.source + " JOIN " + s.database.Statement.Quote(plan.Schema.Name) + " b ON b.`id`=" + lookup.candidate + " FOR UPDATE"
	rows, err := s.database.WithContext(ctx).Raw(query, lookup.arguments...).Rows()
	if err != nil {
		return nil, classifyMutationError(err, ctx.Err())
	}
	defer rows.Close()
	readBytes := 0
	for rows.Next() {
		var ordinal int
		values := make([]*string, len(plan.Schema.Columns))
		dest := []any{&ordinal}
		for i := range values {
			dest = append(dest, &values[i])
		}
		if err := rows.Scan(dest...); err != nil || ordinal < 0 || ordinal >= len(result) {
			return nil, application.ErrReleaseUnavailable
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
				return nil, application.ErrPublicationUnsupported
			}
			if values[i] != nil {
				readBytes += len(*values[i])
				if readBytes > application.ReleaseResultBytes {
					return nil, application.ErrReleaseResultLimit
				}
			}
			fields = append(fields, domain.CanonicalField{Name: column.Name, Type: types[column.Name], Encoding: encoding, Value: values[i]})
		}
		row, err := domain.NewCanonicalRow(plan.SchemaDigest, false, fields)
		if err != nil {
			return nil, application.ErrPublicationUnsupported
		}
		result[ordinal] = row
	}
	if rows.Err() != nil {
		return nil, application.ErrReleaseUnavailable
	}
	return result, nil
}

var _ application.PublicationSession = (*publicationSession)(nil)
var _ application.ReleaseOrderStore = (*Adapter)(nil)

// LockUnchangedPublication compares the current raw MySQL values, including NULL,
// generated and audit fields. Record Versions alone cannot observe external SQL.
// These row/gap locks remain held until quick restoration and target release commit.
func (s *publicationSession) LockUnchangedPublication(ctx context.Context, original domain.ReleaseOrder, schema domain.TableSchema) error {
	if original.Publication == nil || original.VerifyPublication() != nil {
		return application.ErrReleaseUnavailable
	}
	encoded, _ := json.Marshal(original.Frozen.Schema)
	sum := sha256.Sum256(encoded)
	plan := application.PublicationPlan{Schema: schema, Execution: original.Frozen.Schema, SchemaDigest: hex.EncodeToString(sum[:])}
	ids := make([]any, len(original.Publication.Commands))
	for i, command := range original.Publication.Commands {
		ids[i] = command.ID
	}
	rows, err := s.readCanonicalRows(ctx, plan, ids)
	if err != nil {
		return err
	}
	for i, row := range rows {
		if row.Checksum != original.Publication.Commands[i].Final.Checksum {
			return &application.ReleaseItemError{Index: len(rows) - 1 - i, Cause: application.ErrRecordVersionConflict}
		}
	}
	return nil
}
