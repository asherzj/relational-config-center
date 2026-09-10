package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrControlSchemaNotReady contains only release-owned, safe migration guidance.
var ErrControlSchemaNotReady = errors.New("schema_not_ready: use schema-migrate status and docs/schema-migrations.md before deployment")

// Ready observes the same migration progress and complete control structure as
// maintenance, using a read-only snapshot. It never initializes Goose metadata.
func (adapter *Adapter) Ready(ctx context.Context) error {
	tx, err := adapter.pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("%w: database snapshot unavailable", ErrControlSchemaNotReady)
	}
	defer tx.Rollback()
	status, err := readControlSchemaStatus(ctx, tx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrControlSchemaNotReady, err)
	}
	if status.State != "current" {
		return fmt.Errorf("%w: %s (current %d, required %d)", ErrControlSchemaNotReady, status.State, status.Current, status.Required)
	}

	if err := checkReadyMigrationMetadata(ctx, tx); err != nil {
		return fmt.Errorf("%w: %v", ErrControlSchemaNotReady, err)
	}
	digest, err := controlSchemaDigest(status.Required)
	if err != nil || status.AttemptDigest != digest {
		return fmt.Errorf("%w: migration release mismatch", ErrControlSchemaNotReady)
	}
	if err := checkControlSchema(ctx, tx, status.Required, false); err != nil {
		return fmt.Errorf("%w: %v", ErrControlSchemaNotReady, err)
	}
	return nil
}

// The bootstrap metadata is not part of a numbered control-schema migration.
// Check the columns and ordering keys used by status/recovery before trusting it.
func checkReadyMigrationMetadata(ctx context.Context, db schemaQuerier) error {
	expected := map[string]string{
		"rcc_goose_db_version":          "id:bigint unsigned:NO:auto_increment,version_id:bigint:NO:,is_applied:tinyint(1):NO:,tstamp:timestamp:YES:DEFAULT_GENERATED",
		"rcc_schema_migration_attempts": "id:bigint unsigned:NO:auto_increment,target_version:bigint:NO:,release_digest:char(64):NO:,state:varchar(16):NO:,recovery_count:bigint unsigned:NO:,started_at:datetime(6):NO:DEFAULT_GENERATED,finished_at:datetime(6):YES:",
	}
	for table, columns := range expected {
		var actual, engine, primary string
		if err := db.QueryRowContext(ctx, `SELECT GROUP_CONCAT(CONCAT(column_name,':',column_type,':',is_nullable,':',extra) ORDER BY ordinal_position SEPARATOR ',') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=?`, table).Scan(&actual); err != nil || actual != columns {
			return errors.New("migration metadata columns are incompatible")
		}
		if err := db.QueryRowContext(ctx, `SELECT engine FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, table).Scan(&engine); err != nil || engine != "InnoDB" {
			return errors.New("migration metadata requires InnoDB")
		}
		if err := db.QueryRowContext(ctx, `SELECT GROUP_CONCAT(CONCAT(index_name,':',column_name) ORDER BY index_name,seq_in_index) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND non_unique=0`, table).Scan(&primary); err != nil || primary != "PRIMARY:id" {
			return errors.New("migration metadata ordering key is incompatible")
		}
	}
	var unconfirmed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rcc_schema_migration_attempts WHERE state NOT IN ('SUCCEEDED','BASELINED')`).Scan(&unconfirmed); err != nil || unconfirmed != 0 {
		return errors.New("migration history contains unconfirmed attempts")
	}
	return nil
}
