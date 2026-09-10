package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pressly/goose/v3/database"
)

// The historical adoption boundary is frozen at Goose 00005. Later releases
// must explicitly upgrade; adding a migration never moves this boundary.
const historicalBaselineVersion int64 = 5

// BaselineControlSchema adopts only the complete historical baseline.
// It uses the migration session lock and journal, but never executes migration SQL.
func (adapter *Adapter) BaselineControlSchema(ctx context.Context, options SchemaMigrationOptions) (status SchemaMigrationStatus, err error) {
	status, err = adapter.ControlSchemaStatus(ctx)
	if err != nil {
		return status, err
	}
	conn, err := adapter.pool.Conn(ctx)
	if err != nil {
		return status, errors.New("baseline_unavailable: cannot open maintenance connection")
	}
	defer conn.Close()
	lock := &schemaMigrationLock{wait: options.LockTimeout, version: min(status.Required, historicalBaselineVersion), baseline: true, recover: options.Recover}
	if options.Recover && status.AttemptID > 0 {
		lock.version = status.AttemptVersion
	}
	err = lock.SessionLock(ctx, conn)
	if err != nil && !errors.Is(err, errSchemaAlreadyApplied) {
		return status, err
	}
	if err == nil {
		err = recordBaselineVersions(ctx, conn, lock.version)
		if unlockErr := lock.SessionUnlock(ctx, conn); unlockErr != nil {
			err = unlockErr
		}
	}
	// Release the only pool connection before the final read-only status query.
	_ = conn.Close()
	if err != nil && !errors.Is(err, errSchemaAlreadyApplied) {
		return status, err
	}
	status, err = adapter.ControlSchemaStatus(ctx)
	if err == nil && status.State != "current" && status.State != "pending" {
		err = errors.New("baseline_incomplete: inspect status and explicitly recover the unfinished adoption")
	}
	return status, err
}

func (lock *schemaMigrationLock) prepareBaseline(ctx context.Context, conn *sql.Conn, status SchemaMigrationStatus, digest string) error {
	if status.State == "current" || status.State == "pending" {
		return errSchemaAlreadyApplied
	}
	if status.State == "recovery_required" {
		if !lock.recover {
			return errors.New("recovery_required: inspect status and actual schema, then explicitly run recover")
		}
		if status.AttemptID == 0 {
			if status.Current != 0 {
				return errors.New("recovery_unverified: missing baseline attempt with existing versions requires manual investigation")
			}
			// No control DDL is ever executed before its journal entry. A complete
			// current structure can therefore only be adopted after revalidation.
			return lock.beginBaseline(ctx, conn, digest)
		}
		if status.AttemptOperation != "baseline" || status.AttemptDigest != digest {
			return errors.New("recovery_release_mismatch: restore the original baseline release before recovery")
		}
		if status.Current != 0 && status.Current != lock.version {
			return errors.New("recovery_unverified: incomplete baseline version prefix requires manual investigation")
		}
		var target int64
		if err := conn.QueryRowContext(ctx, `SELECT target_version FROM rcc_schema_migration_attempts WHERE id=? AND state='BASELINING'`, status.AttemptID).Scan(&target); err != nil || target != lock.version {
			return errors.New("recovery_unverified: only an unfinished baseline attempt can be recovered; investigate missing or changed history")
		}
		if err := checkControlSchema(ctx, conn, lock.version, false); err != nil {
			return err
		}
		result, err := conn.ExecContext(ctx, `UPDATE rcc_schema_migration_attempts SET recovery_count=recovery_count+1 WHERE id=? AND state='BASELINING'`, status.AttemptID)
		if err != nil {
			return errors.New("recovery_unavailable: cannot record explicit baseline recovery")
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return errors.New("recovery_unverified: baseline attempt changed during verification")
		}
		lock.attemptID = status.AttemptID
		if status.Current == lock.version {
			return errSchemaAlreadyApplied
		}
		if err := initializeSchemaVersionZero(ctx, conn); err != nil {
			return errors.New("baseline_failed: version table initialization incomplete; inspect status and explicitly recover")
		}
		return nil
	}
	return lock.beginBaseline(ctx, conn, digest)
}

func (lock *schemaMigrationLock) beginBaseline(ctx context.Context, conn *sql.Conn, digest string) error {
	if err := checkControlSchema(ctx, conn, lock.version, false); err != nil {
		return errors.New(err.Error() + "; follow deploy/mysql/migrations/README.md to reach the current historical baseline before adoption")
	}
	if _, err := conn.ExecContext(ctx, schemaAttemptDDL); err != nil {
		return errors.New("baseline_failed: cannot initialize attempt tracking; inspect status before retrying")
	}
	result, err := conn.ExecContext(ctx, `INSERT INTO rcc_schema_migration_attempts(target_version,release_digest,state) VALUES (?,?,'BASELINING')`, lock.version, digest)
	if err != nil {
		return errors.New("baseline_failed: cannot record attempt; inspect status before retrying")
	}
	lock.attemptID, err = result.LastInsertId()
	if err != nil {
		return errors.New("baseline_failed: attempt result unknown; inspect status before retrying")
	}
	if err := initializeSchemaVersionZero(ctx, conn); err != nil {
		return errors.New("baseline_failed: version table initialization incomplete; inspect status and explicitly recover")
	}
	return nil
}

func recordBaselineVersions(ctx context.Context, conn *sql.Conn, target int64) error {
	versions, err := controlSchemaVersions()
	if err != nil {
		return err
	}
	store, err := database.NewStore(database.DialectMySQL, schemaVersionTable)
	if err != nil {
		return errors.New("baseline_invalid: cannot load version store")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("baseline_failed: cannot begin version registration; inspect status and explicitly recover")
	}
	defer tx.Rollback()
	for _, version := range versions {
		if version > target {
			break
		}
		if err := store.Insert(ctx, tx, database.InsertRequest{Version: version}); err != nil {
			return errors.New("baseline_failed: cannot register baseline versions; inspect status and explicitly recover")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.New("baseline_result_unknown: version registration outcome unknown; inspect status and explicitly recover")
	}
	return nil
}
