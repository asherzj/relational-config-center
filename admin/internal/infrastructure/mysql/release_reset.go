package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrReleaseResetTarget = errors.New("release reset target does not match connected database")
	ErrReleaseResetSchema = errors.New("release reset requires the supported schema without triggers or foreign keys")
)

// The order is explicit; every historical request result is removed, including
// old workflow snapshots that do not have an order_id column.
var releaseResetTables = []string{"rcc_release_requests", "rcc_release_targets", "rcc_publication_commands", "rcc_refresh_notifications", "rcc_release_orders"}

type ReleaseResetFingerprint struct {
	Rows   uint64 `json:"rows"`
	SHA256 string `json:"sha256"`
}

type ReleaseResetReport struct {
	Database        string                             `json:"database"`
	ServerUUID      string                             `json:"server_uuid"`
	Committed       bool                               `json:"committed"`
	Before          map[string]uint64                  `json:"before"`
	After           map[string]uint64                  `json:"after"`
	PreservedBefore map[string]ReleaseResetFingerprint `json:"preserved_before"`
	PreservedAfter  map[string]ReleaseResetFingerprint `json:"preserved_after"`
}

// ResetReleaseHistory is an offline dev/test maintenance operation, never a
// startup or migration hook. The caller must identify the target and stop and
// drain all writers (including pending retries) before invoking it.
func (a *Adapter) ResetReleaseHistory(ctx context.Context, database, serverUUID string) (ReleaseResetReport, error) {
	report := ReleaseResetReport{}
	if database == "" || serverUUID == "" || database != a.database {
		return report, ErrReleaseResetTarget
	}
	err := a.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw("SELECT DATABASE(),@@server_uuid").Row().Scan(&report.Database, &report.ServerUUID); err != nil {
			return err
		}
		if report.Database != database || report.ServerUUID != serverUUID {
			return ErrReleaseResetTarget
		}
		versions, err := controlSchemaVersions()
		if err != nil {
			return ErrReleaseResetSchema
		}
		manifest, err := controlSchemaManifest(versions[len(versions)-1])
		if err != nil {
			return ErrReleaseResetSchema
		}
		// Keep metadata locks until commit so schema checks remain valid for deletes.
		// Reuse the release definition only for tables this operation touches. This
		// maintenance command does not require Goose adoption or HTTP readiness.
		for _, table := range append(append([]string{}, releaseResetTables...), "rcc_record_versions", "rcc_table_publications") {
			rows, err := tx.Raw("SELECT * FROM `" + table + "` LIMIT 0").Rows()
			if err != nil {
				return ErrReleaseResetSchema
			}
			if err = rows.Close(); err != nil {
				return err
			}
			var name, actual string
			if err := tx.Raw("SHOW CREATE TABLE `"+table+"`").Row().Scan(&name, &actual); err != nil || comparableControlDefinition(actual) != comparableControlDefinition(manifest[table]) {
				return ErrReleaseResetSchema
			}
		}
		for _, table := range releaseResetTables {
			if err := releaseResetSideEffects(tx, table); err != nil {
				return err
			}
		}
		report.Before, err = releaseResetCounts(tx)
		if err != nil {
			return err
		}
		report.PreservedBefore, err = releaseResetPreserved(tx)
		if err != nil {
			return err
		}
		for _, table := range releaseResetTables {
			if err := tx.Exec("DELETE FROM `" + table + "`").Error; err != nil {
				return err
			}
		}
		report.After, err = releaseResetCounts(tx)
		if err != nil {
			return err
		}
		for _, count := range report.After {
			if count != 0 {
				return errors.New("release reset verification failed")
			}
		}
		report.PreservedAfter, err = releaseResetPreserved(tx)
		if err != nil {
			return err
		}
		for table, before := range report.PreservedBefore {
			if before != report.PreservedAfter[table] {
				return errors.New("release reset changed preserved versions")
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ReleaseResetReport{}, err
	}
	report.Committed = true
	return report, nil
}

func releaseResetCounts(tx *gorm.DB) (map[string]uint64, error) {
	result := map[string]uint64{}
	for _, table := range releaseResetTables {
		var count uint64
		if err := tx.Raw("SELECT COUNT(*) FROM `" + table + "`").Row().Scan(&count); err != nil {
			return nil, err
		}
		result[table] = count
	}
	return result, nil
}

func releaseResetPreserved(tx *gorm.DB) (map[string]ReleaseResetFingerprint, error) {
	result := map[string]ReleaseResetFingerprint{}
	for _, item := range []struct{ table, query string }{
		{"rcc_record_versions", "SELECT HEX(table_name),HEX(record_key),lock_version FROM rcc_record_versions ORDER BY table_name,record_key"},
		{"rcc_table_publications", "SELECT HEX(table_name),table_version,command_cursor FROM rcc_table_publications ORDER BY table_name"},
	} {
		rows, err := tx.Raw(item.query).Rows()
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		encoder := json.NewEncoder(hash)
		var count uint64
		for rows.Next() {
			var values [3]string
			if err = rows.Scan(&values[0], &values[1], &values[2]); err != nil {
				break
			}
			if err = encoder.Encode(values); err != nil {
				break
			}
			count++
		}
		rowErr := rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return nil, err
		}
		if rowErr != nil {
			return nil, rowErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		result[item.table] = ReleaseResetFingerprint{Rows: count, SHA256: hex.EncodeToString(hash.Sum(nil))}
	}
	return result, nil
}

func releaseResetSideEffects(tx *gorm.DB, table string) error {
	granted, err := directTriggerGrant(tx, table)
	if err != nil {
		return err
	}
	if !granted {
		return ErrReleaseResetSchema
	}
	var triggers, foreignKeys int
	if err := tx.Raw(`SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE EVENT_OBJECT_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE=?`, table).Row().Scan(&triggers); err != nil {
		return err
	}
	// Global InnoDB metadata requires PROCESS, including references from schemas
	// the maintenance identity cannot otherwise read. Never disable FK checks.
	if err := tx.Raw(`SELECT COUNT(*) FROM information_schema.INNODB_FOREIGN WHERE FOR_NAME=CONCAT(DATABASE(),'/',?) OR REF_NAME=CONCAT(DATABASE(),'/',?)`, table, table).Row().Scan(&foreignKeys); err != nil {
		return fmt.Errorf("release reset metadata unavailable: %w", err)
	}
	if triggers != 0 || foreignKeys != 0 {
		return ErrReleaseResetSchema
	}
	return nil
}
