package mysql

import (
	"context"
	"crypto/sha256"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// The ordinal belongs to the request, while weights belong to MySQL. A join
// therefore preserves each requested slot without assuming result row order.
func (s *releaseOrderSession) ReadRecordBaselines(ctx context.Context, schema domain.TableSchema, ids []any) ([]domain.RecordBaseline, error) {
	return s.readRecordBaselines(ctx, schema, ids, nil)
}

// Only a verified original DELETE can supply an absent ENUM's comparison key.
// Ordinary ADD continues to reject unsupported missing-ENUM identity synthesis.
func (s *releaseOrderSession) ReadRollbackBaselines(ctx context.Context, schema domain.TableSchema, ids []any, source domain.ReleaseOrder) ([]domain.RecordBaseline, error) {
	if source.VerifyPublication() != nil || source.Publication == nil || len(source.Items) != len(ids) {
		return nil, application.ErrReleaseUnavailable
	}
	return s.readRecordBaselines(ctx, schema, ids, &source)
}

func (s *releaseOrderSession) readRecordBaselines(ctx context.Context, schema domain.TableSchema, ids []any, source *domain.ReleaseOrder) ([]domain.RecordBaseline, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	result := make([]domain.RecordBaseline, len(ids))
	for _, c := range schema.Columns {
		if c.Type == domain.ColumnTypeUnsupported {
			return nil, application.ErrReleaseSnapshotUnsupported
		}
	}
	meta, err := recordIdentityMetadata(ctx, s.database, schema.Name)
	if err != nil {
		return nil, err
	}
	lookup, err := buildRecordLookup(meta, ids)
	if err != nil {
		return nil, err
	}
	if len(lookup.arguments) == 0 {
		return result, nil
	}
	candidate := lookup.candidate
	weight := "IF(b.`id` IS NULL,WEIGHT_STRING(" + recordWeightExpression(meta, candidate) + "),WEIGHT_STRING(" + recordWeightExpression(meta, "b.`id`") + "))"
	projections := []string{"requested.ordinal", weight, "b.`id` IS NOT NULL"}
	for _, column := range schema.Columns {
		field := "b." + s.database.Statement.Quote(column.Name)
		if column.Type == domain.ColumnTypeFloat64 {
			field = "CAST(" + field + " AS DOUBLE)"
		}
		projections = append(projections, field)
	}
	query := "SELECT " + strings.Join(projections, ",") + " FROM " + lookup.source + " LEFT JOIN " + s.database.Statement.Quote(schema.Name) + " b ON b.`id`=" + candidate
	rows, err := s.database.WithContext(ctx).Raw(query, lookup.arguments...).Rows()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	keys := [][]byte{}
	readBytes := 0
	defer rows.Close()
	for rows.Next() {
		var index int
		var weight []byte
		var exists bool
		values := make([]any, len(schema.Columns))
		dest := []any{&index, &weight, &exists}
		for i := range values {
			dest = append(dest, &values[i])
		}
		if rows.Scan(dest...) != nil || index < 0 || index >= len(result) || weight == nil {
			return nil, application.ErrReleaseUnavailable
		}
		sum := sha256.Sum256(weight)
		key := sum[:]
		if !exists && meta.DataType == "enum" {
			if source == nil {
				return nil, &application.ReleaseItemError{Index: index, Cause: application.ErrReleaseSnapshotUnsupported}
			}
			// Reverse inputs unwind the original execution order.
			sourceIndex := len(source.Items) - 1 - index
			saved := source.Items[sourceIndex]
			command := source.Publication.Commands[sourceIndex]
			if command.Operation != "DELETE" || saved.RecordTable != meta.TableName || len(saved.RecordKey) != 32 || !sameRollbackEnumDefinition(source.Frozen.Schema, meta) {
				return nil, &application.ReleaseItemError{Index: index, Cause: application.ErrRecordVersionConflict}
			}
			key = saved.RecordKey
		}
		baseline := domain.RecordBaseline{TableName: meta.TableName, Key: key, Version: "0"}
		keys = append(keys, key)
		if exists {
			baseline.Row = domain.Row{}
			for i, column := range schema.Columns {
				value := jsonStringCell(column, values[i])
				if value != nil {
					readBytes += len(*value)
					if readBytes > application.ReleaseResultBytes {
						return nil, application.ErrReleaseResultLimit
					}
				}
				if value != nil && !utf8.ValidString(string(*value)) {
					return nil, &application.ReleaseItemError{Index: index, Cause: application.ErrReleaseSnapshotUnsupported}
				}
				baseline.Row[column.Name] = value
			}
		}
		result[index] = baseline
	}
	if rows.Err() != nil {
		return nil, application.ErrReleaseUnavailable
	}
	rows.Close()
	var noAutoZero bool
	if column, ok := schema.Column("id"); ok && column.AutoIncrement {
		if s.database.WithContext(ctx).Raw("SELECT FIND_IN_SET('NO_AUTO_VALUE_ON_ZERO',@@session.sql_mode)>0").Row().Scan(&noAutoZero) != nil {
			return nil, application.ErrReleaseUnavailable
		}
	}
	versionRows, err := s.database.WithContext(ctx).Raw("SELECT record_key,lock_version FROM rcc_record_versions WHERE table_name=? AND (record_key IN ? OR record_key=X'')", meta.TableName, keys).Rows()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	defer versionRows.Close()
	versions := map[string]uint64{}
	var floor uint64
	for versionRows.Next() {
		var key []byte
		var version uint64
		if versionRows.Scan(&key, &version) != nil {
			return nil, application.ErrReleaseUnavailable
		}
		if len(key) == 0 {
			floor = version
		} else {
			versions[string(key)] = version
		}
	}
	if versionRows.Err() != nil {
		return nil, application.ErrReleaseUnavailable
	}
	for i, baseline := range result {
		if len(baseline.Key) > 0 {
			version := versions[string(baseline.Key)]
			if version < floor {
				version = floor
			}
			result[i].Version = strconv.FormatUint(version, 10)
			if column, ok := schema.Column("id"); ok && column.AutoIncrement && !noAutoZero {
				switch value := ids[i].(type) {
				case int64:
					result[i].GeneratesIDOnInsert = value == 0
				case uint64:
					result[i].GeneratesIDOnInsert = value == 0
				}
			}
		}
	}
	return result, nil
}

func sameRollbackEnumDefinition(schema domain.TableExecutionSchema, meta identityMetadata) bool {
	columns, err := schema.Columns()
	if err != nil {
		return false
	}
	for _, column := range columns {
		if column.Name == "id" {
			return column.Type == meta.ColumnType && column.Charset != nil && *column.Charset == meta.CharsetName && column.Collation != nil && *column.Collation == meta.CollationName
		}
	}
	return false
}
