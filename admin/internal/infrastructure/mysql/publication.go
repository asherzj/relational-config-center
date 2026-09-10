package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"slices"
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
	// Drafts hold this policy row FOR SHARE before their first consistent read.
	// The exclusive guard keeps business writes and target release from crossing
	// a saved baseline, while independent draft saves may still run together.
	var queryCode, mutationCode string
	if err := s.database.WithContext(ctx).Raw(`SELECT query_policy_code,mutation_policy_code FROM rcc_table_policies WHERE table_name=? AND BINARY table_name=BINARY ? FOR UPDATE`, table, table).Row().Scan(&queryCode, &mutationCode); err != nil {
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

// A table batch is used only for metadata, locked row reads, versions and
// notifications. Business DML always visits plan.Items in its frozen order.
type publicationTableBatch struct {
	table           application.PublicationTable
	positions       []int
	items           []application.PublicationItem
	ids             []any
	before, final   []domain.CanonicalRow
	baselines       []domain.RecordBaseline
	versions        []string
	version, cursor uint64
}

func publicationGroups(plan application.PublicationPlan) (map[string]*publicationTableBatch, []string) {
	groups := map[string]*publicationTableBatch{}
	for index, item := range plan.Items {
		name := item.Intent.TableName
		if groups[name] == nil {
			groups[name] = &publicationTableBatch{table: plan.Tables[name]}
		}
		group := groups[name]
		group.positions = append(group.positions, index)
		group.items = append(group.items, item)
		group.ids = append(group.ids, item.ID)
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	slices.Sort(names)
	return groups, names
}

func (s *publicationSession) CommitPublication(ctx context.Context, plan application.PublicationPlan) (domain.PublicationCommit, error) {
	fail := func(err error) (domain.PublicationCommit, error) { return domain.PublicationCommit{}, err }
	if err := s.available(); err != nil {
		return fail(err)
	}
	if len(plan.Items) == 0 || len(plan.Items) > 1000 {
		return fail(application.ErrReleaseItemLimit)
	}
	groups, tables := publicationGroups(plan)
	for _, name := range tables {
		group := groups[name]
		if err := s.checkPublicationCapability(ctx, group.table); err != nil {
			return fail(application.RemapReleaseItemError(err, group.positions))
		}
		if err := s.database.WithContext(ctx).Raw(`SELECT table_version,command_cursor FROM rcc_table_publications WHERE table_name=? FOR UPDATE`, name).Row().Scan(&group.version, &group.cursor); err != nil {
			return fail(application.ErrReleaseUnavailable)
		}
		if group.version == math.MaxUint64 || uint64(len(group.items)) > math.MaxUint64-group.cursor {
			return fail(application.ErrReleaseUnavailable)
		}
		group.version++
		var err error
		group.before, err = s.readCanonicalRows(ctx, group.table, group.ids)
		if err != nil {
			return fail(application.RemapReleaseItemError(err, group.positions))
		}
	}
	// The sole DML loop: never regroup by table or infer foreign-key dependencies.
	localPositions := map[string]int{}
	for index, item := range plan.Items {
		group := groups[item.Intent.TableName]
		local := localPositions[item.Intent.TableName]
		localPositions[item.Intent.TableName]++
		before := group.before[local]
		if item.Intent.Operation == "ADD" && !before.Deleted {
			return fail(&application.ReleaseItemError{Index: index, Cause: application.ErrDuplicateKey})
		}
		if item.Intent.Operation != "ADD" && before.Deleted {
			return fail(&application.ReleaseItemError{Index: index, Cause: application.ErrMutationRowNotFound})
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
				changed = db.Exec("INSERT INTO " + db.Statement.Quote(group.table.Schema.Name) + " VALUES ()")
			} else {
				changed = db.Table(group.table.Schema.Name).Create(values)
			}
		case "MODIFY":
			changed = db.Table(group.table.Schema.Name).Where("`id`=?", group.ids[local]).Updates(values)
		case "DELETE":
			changed = db.Table(group.table.Schema.Name).Where("`id`=?", group.ids[local]).Delete(&map[string]any{})
		default:
			return fail(application.ErrInvalidMutation)
		}
		if changed.Error != nil {
			return fail(&application.ReleaseItemError{Index: index, Cause: classifyMutationError(changed.Error, ctx.Err())})
		}
		if changed.RowsAffected != 1 {
			return fail(&application.ReleaseItemError{Index: index, Cause: application.ErrMutationRowNotFound})
		}
		if item.Intent.Operation == "ADD" && group.ids[local] == nil {
			if insertResult.Result == nil {
				return fail(application.ErrReleaseUnavailable)
			}
			actual, err := insertResult.Result.LastInsertId()
			if err != nil {
				return fail(application.ErrReleaseUnavailable)
			}
			group.ids[local] = strconv.FormatUint(uint64(actual), 10)
		}
	}
	executionID := plan.OrderID + ":" + plan.ExecutionKind
	result := domain.PublicationCommit{ExecutionID: executionID, Kind: plan.ExecutionKind, PublisherID: plan.PublisherID, ExecutedAt: plan.At.UTC().Format(time.RFC3339Nano), Commands: make([]domain.PublicationCommand, len(plan.Items)), TableVersions: map[string]string{}, Notifications: map[string]domain.RefreshNotification{}}
	generated := []domain.ActiveTarget{}
	for _, name := range tables {
		group := groups[name]
		var err error
		group.final, err = s.readCanonicalRows(ctx, group.table, group.ids)
		if err != nil {
			return fail(application.RemapReleaseItemError(err, group.positions))
		}
		identityIDs := append([]any(nil), group.ids...)
		for local, item := range group.items {
			if item.Intent.Operation == "DELETE" {
				identityIDs[local] = nil
			}
		}
		group.baselines, err = s.ReadRecordBaselines(ctx, group.table.Schema, identityIDs)
		if err != nil {
			return fail(application.RemapReleaseItemError(err, group.positions))
		}
		for local, item := range group.items {
			if item.Intent.Operation == "DELETE" {
				group.baselines[local] = domain.RecordBaseline{TableName: item.Intent.RecordTable, Key: item.Intent.RecordKey, Version: item.Intent.ExpectedRecordVersion}
			}
			baseline := group.baselines[local]
			if item.Intent.ID != nil && (!bytes.Equal(item.Intent.RecordKey, baseline.Key) || item.Intent.RecordTable != baseline.TableName) {
				return fail(&application.ReleaseItemError{Index: group.positions[local], Cause: application.ErrRecordVersionConflict})
			}
			if item.Intent.ID == nil {
				generated = append(generated, domain.ActiveTarget{ItemIndex: group.positions[local], TableName: baseline.TableName, RecordKey: baseline.Key})
			}
		}
	}
	if err := s.ReserveReleaseTargets(ctx, plan.OrderID, generated); err != nil {
		return fail(err)
	}
	for _, name := range tables {
		group := groups[name]
		var err error
		group.versions, err = s.advancePublicationVersions(ctx, plan, name, group.items, group.baselines)
		if err != nil {
			return fail(application.RemapReleaseItemError(err, group.positions))
		}
		version := strconv.FormatUint(group.version, 10)
		result.TableVersions[name] = version
		result.Notifications[name] = domain.RefreshNotification{ID: executionID, TableName: name, TableVersion: version, Status: "NOT_CONNECTED"}
		for local, item := range group.items {
			before, final := group.before[local], group.final[local]
			identity := final
			if final.Deleted {
				identity = before
			}
			actualID, err := identity.RecordID()
			if err != nil || final.Deleted != (item.Intent.Operation == "DELETE") {
				return fail(&application.ReleaseItemError{Index: group.positions[local], Cause: application.ErrPublicationUnsupported})
			}
			column, found := group.table.Schema.Column("id")
			if !found {
				return fail(application.ErrPublicationUnsupported)
			}
			if _, err := domain.ParseColumnValue(column, domain.JSONString(actualID)); err != nil {
				return fail(&application.ReleaseItemError{Index: group.positions[local], Cause: application.ErrInvalidMutation})
			}
			group.cursor++
			result.Commands[group.positions[local]] = domain.PublicationCommand{DetailID: item.Intent.DetailID, ExecutionID: executionID, ExecutionKind: plan.ExecutionKind, OrderID: plan.OrderID, Sequence: strconv.FormatUint(group.cursor, 10), TableName: name, TableVersion: version, Operation: item.Intent.Operation, ID: actualID, RecordVersion: group.versions[local], Before: before, Final: final}
		}
		if err := s.database.WithContext(ctx).Exec(`UPDATE rcc_table_publications SET table_version=?,command_cursor=? WHERE table_name=?`, group.version, group.cursor, name).Error; err != nil {
			return fail(application.ErrReleaseUnavailable)
		}
		notice, err := json.Marshal(result.Notifications[name])
		if err != nil {
			return fail(application.ErrReleaseUnavailable)
		}
		if err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_refresh_notifications(order_id,execution_id,table_name,table_version,document) VALUES(?,?,?,?,?)`, plan.OrderID, executionID, name, group.version, notice).Error; err != nil {
			return fail(application.ErrReleaseUnavailable)
		}
	}
	// Batch storage round trips without a whole-order byte ceiling. A large
	// individual result still gets its own statement; every batch shares this TX.
	var values []string
	var arguments []any
	encodedBytes := 0
	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_publication_commands(table_name,sequence,order_id,execution_id,document) VALUES`+strings.Join(values, ","), arguments...).Error
		values, arguments, encodedBytes = nil, nil, 0
		return err
	}
	for _, command := range result.Commands {
		encoded, err := json.Marshal(command)
		if err != nil {
			return fail(application.ErrReleaseUnavailable)
		}
		if len(values) == 100 || encodedBytes+len(encoded) > 1<<20 {
			if err := flush(); err != nil {
				return fail(application.ErrReleaseUnavailable)
			}
		}
		values = append(values, "(?,?,?,?,?)")
		arguments = append(arguments, command.TableName, command.Sequence, plan.OrderID, executionID, encoded)
		encodedBytes += len(encoded)
	}
	if err := flush(); err != nil {
		return fail(application.ErrReleaseUnavailable)
	}
	// Bounded single-table display aliases are retired with the T7 API facade.
	return result, nil
}

func (s *publicationSession) readCanonicalRows(ctx context.Context, plan application.PublicationTable, ids []any) ([]domain.CanonicalRow, error) {
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
func (s *publicationSession) LockUnchangedPublication(ctx context.Context, original domain.ReleaseOrder, tables map[string]application.PublicationTable) error {
	if len(original.Executions) == 0 || original.VerifyPublication() != nil {
		return application.ErrReleaseUnavailable
	}
	plan := application.PublicationPlan{Tables: tables}
	for _, detail := range original.Items {
		command := detail.Publication
		plan.Items = append(plan.Items, application.PublicationItem{Intent: domain.ReleaseItem{TableName: command.TableName}, ID: command.ID})
	}
	groups, names := publicationGroups(plan)
	for _, name := range names {
		group := groups[name]
		rows, err := s.readCanonicalRows(ctx, group.table, group.ids)
		if err != nil {
			return application.ReverseReleaseItemError(application.RemapReleaseItemError(err, group.positions), len(plan.Items))
		}
		for local, row := range rows {
			index := group.positions[local]
			if row.Checksum != original.Items[index].Publication.Final.Checksum {
				return &application.ReleaseItemError{Index: len(plan.Items) - 1 - index, Cause: application.ErrRecordVersionConflict}
			}
		}
	}
	return nil
}
