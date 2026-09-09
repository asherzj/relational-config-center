package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// MySQL produces each typed component's equality weight. Length framing and a
// NULL marker preserve component boundaries, including NULL versus empty text.
// The supplementary namespace cannot alias the unframed primary-key identity.
func (s *releaseOrderSession) ReadConcurrencyKeys(ctx context.Context, schema domain.TableSchema, fields []string, values []domain.Row) ([][]byte, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	metas := make([]identityMetadata, len(fields))
	expressions := make([]string, len(fields))
	for i, name := range fields {
		meta, err := columnIdentityMetadata(ctx, s.database, schema.Name, name)
		if err != nil {
			return nil, err
		}
		metas[i] = meta
		expression, err := recordLookupExpression(meta)
		if err != nil {
			return nil, application.ErrConcurrencyKeyInvalid
		}
		expressions[i] = "WEIGHT_STRING(" + recordWeightExpression(meta, strings.ReplaceAll(expression, "?", fmt.Sprintf("v%d", i))) + ")"
	}
	keys := make([][]byte, len(values))
	for start := 0; start < len(values); start += 100 {
		end := min(start+100, len(values))
		sources := []string{}
		args := []any{}
		for index := start; index < end; index++ {
			parts := []string{"? AS ordinal"}
			args = append(args, index)
			for field, name := range fields {
				cell, supplied := values[index][name]
				column, _ := schema.Column(name)
				if !supplied || cell == nil && !column.Nullable {
					return nil, &application.ReleaseItemError{Index: index, Cause: application.ErrConcurrencyKeyValue}
				}
				var value any
				if cell != nil {
					var err error
					// Content was validated during preparation. Persisted MySQL TIME
					// can additionally contain signed durations from the real old row.
					if column.Type == domain.ColumnTypeTime {
						value = string(*cell)
					} else {
						value, err = domain.ParseColumnValue(column, *cell)
					}
					if err != nil || metas[field].Capacity > 0 && len([]rune(string(*cell))) > metas[field].Capacity {
						return nil, &application.ReleaseItemError{Index: index, Cause: application.ErrConcurrencyKeyValue}
					}
				}
				parts = append(parts, fmt.Sprintf("? AS v%d", field))
				args = append(args, value)
			}
			sources = append(sources, "SELECT "+strings.Join(parts, ","))
		}
		rows, err := s.database.WithContext(ctx).Raw("SELECT ordinal,"+strings.Join(expressions, ",")+" FROM ("+strings.Join(sources, " UNION ALL ")+") candidates", args...).Rows()
		if err != nil {
			return nil, application.ErrReleaseUnavailable
		}
		for rows.Next() {
			var index int
			weights := make([][]byte, len(fields))
			dest := []any{&index}
			for i := range weights {
				dest = append(dest, &weights[i])
			}
			if err := rows.Scan(dest...); err != nil || index < start || index >= end {
				rows.Close()
				return nil, application.ErrReleaseUnavailable
			}
			encoded := []byte("rcc-concurrency-key-v1\x00")
			for field, name := range fields {
				encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(name)))
				encoded = append(encoded, []byte(name)...)
				if values[index][name] == nil {
					encoded = append(encoded, 0)
					continue
				}
				if weights[field] == nil {
					rows.Close()
					return nil, &application.ReleaseItemError{Index: index, Cause: application.ErrConcurrencyKeyValue}
				}
				encoded = append(encoded, 1)
				encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(weights[field])))
				encoded = append(encoded, weights[field]...)
			}
			sum := sha256.Sum256(encoded)
			keys[index] = sum[:]
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, application.ErrReleaseUnavailable
		}
	}
	return keys, nil
}
