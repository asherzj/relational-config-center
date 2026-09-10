package mysql

import (
	"bytes"
	"context"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Lock the complete key set, compare every observed version, then advance all
// keys in the publication transaction. Any mismatch rolls back the earlier DML.
func (s *publicationSession) advancePublicationVersions(ctx context.Context, plan application.PublicationPlan, table string, items []application.PublicationItem, baselines []domain.RecordBaseline) ([]string, error) {
	positions := make([]int, len(baselines))
	keys := make([][]byte, len(baselines))
	for i, b := range baselines {
		positions[i] = i
		keys[i] = b.Key
	}
	sort.Slice(positions, func(i, j int) bool { return bytes.Compare(keys[positions[i]], keys[positions[j]]) < 0 })
	var floor uint64
	if s.database.WithContext(ctx).Raw("SELECT COALESCE(MAX(lock_version),0) FROM rcc_record_versions WHERE table_name=? AND record_key=X''", table).Row().Scan(&floor) != nil {
		return nil, application.ErrReleaseUnavailable
	}
	targets, err := s.database.WithContext(ctx).Raw("SELECT record_key,order_id FROM rcc_release_targets WHERE table_name=? AND record_key IN ? ORDER BY record_key FOR UPDATE", table, keys).Rows()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	owners := map[string]string{}
	for targets.Next() {
		var key []byte
		var owner string
		if targets.Scan(&key, &owner) != nil {
			targets.Close()
			return nil, application.ErrReleaseUnavailable
		}
		owners[string(key)] = owner
	}
	err = targets.Err()
	targets.Close()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	for i, key := range keys {
		owner := owners[string(key)]
		// Quick rollback consumes the original reservation inside this transaction.
		// A lost reservation must fail, not be silently recreated or ignored.
		if plan.TargetOrderID != "" {
			if owner != plan.TargetOrderID {
				return nil, &application.ReleaseItemError{Index: i, Cause: application.ErrReleaseTargetConflict}
			}
		} else if owner != "" && owner != plan.OrderID {
			return nil, &application.ReleaseItemError{Index: i, Cause: application.ErrReleaseTargetConflict}
		}
	}
	slots := []string{}
	args := []any{}
	for _, i := range positions {
		slots = append(slots, "(?,?,?)")
		args = append(args, table, keys[i], floor)
	}
	if err := s.database.WithContext(ctx).Exec("INSERT INTO rcc_record_versions(table_name,record_key,lock_version) VALUES"+strings.Join(slots, ",")+" ON DUPLICATE KEY UPDATE record_key=record_key", args...).Error; err != nil {
		return nil, classifyMutationError(err, ctx.Err())
	}
	rows, err := s.database.WithContext(ctx).Raw("SELECT record_key,lock_version FROM rcc_record_versions WHERE table_name=? AND record_key IN ? FOR UPDATE", table, keys).Rows()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	current := map[string]uint64{}
	for rows.Next() {
		var key []byte
		var version uint64
		if rows.Scan(&key, &version) != nil {
			rows.Close()
			return nil, application.ErrReleaseUnavailable
		}
		if version < floor {
			version = floor
		}
		current[string(key)] = version
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, application.ErrReleaseUnavailable
	}
	versions := make([]string, len(keys))
	for i, key := range keys {
		version, exists := current[string(key)]
		expected := items[i].Intent.ExpectedRecordVersion
		if items[i].Intent.ID == nil {
			expected = baselines[i].Version
		}
		if !exists || version == math.MaxUint64 || strconv.FormatUint(version, 10) != expected {
			return nil, &application.ReleaseItemError{Index: i, Cause: application.ErrRecordVersionConflict}
		}
		versions[i] = strconv.FormatUint(version+1, 10)
	}
	changed := s.database.WithContext(ctx).Exec("UPDATE rcc_record_versions SET lock_version=GREATEST(lock_version,?)+1 WHERE table_name=? AND record_key IN ?", floor, table, keys)
	if changed.Error != nil {
		return nil, classifyMutationError(changed.Error, ctx.Err())
	}
	if changed.RowsAffected != int64(len(keys)) {
		return nil, application.ErrRecordVersionConflict
	}
	return versions, nil
}
