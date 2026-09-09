package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/pressly/goose/v3/database"
)

// The named lock stays on Goose's SQL execution session. Losing that connection
// also stops its statements; an independently-held lock would not guarantee this.
type schemaMigrationLock struct {
	wait      time.Duration
	failure   error
	version   int64
	recover   bool
	attemptID int64
	baseline  bool
}

var errSchemaAlreadyApplied = errors.New("schema migration already applied")

func (lock *schemaMigrationLock) SessionLock(ctx context.Context, conn *sql.Conn) (err error) {
	lock.failure = nil
	lock.attemptID = 0
	waitCtx, cancel := context.WithTimeout(ctx, lock.wait)
	defer cancel()
	contended := false
	for {
		var acquired sql.NullInt64
		if err := conn.QueryRowContext(waitCtx, `SELECT GET_LOCK(CONCAT('rcc.schema:',LEFT(SHA2(DATABASE(),256),48)),0)`).Scan(&acquired); err != nil {
			lock.failure = errors.New("migration_lock_unavailable: check database connection and permissions")
			if contended && waitCtx.Err() != nil && ctx.Err() == nil {
				lock.failure = errors.New("migration_busy: another maintenance session holds the lock; retry after it finishes")
			}
			return lock.failure
		}
		if acquired.Valid && acquired.Int64 == 1 {
			break
		}
		contended = acquired.Valid && acquired.Int64 == 0
		select {
		case <-waitCtx.Done():
			lock.failure = errors.New("migration_busy: another maintenance session holds the lock; retry after it finishes")
			return lock.failure
		case <-time.After(50 * time.Millisecond):
		}
	}
	defer func() {
		if err != nil {
			if cleanupErr := lock.SessionUnlock(ctx, conn); cleanupErr != nil {
				err = cleanupErr
			}
			lock.failure = err
		}
	}()
	status, err := readControlSchemaStatus(ctx, conn)
	if err != nil {
		return err
	}
	digest, err := controlSchemaDigest(lock.version)
	if err != nil {
		return err
	}
	if status.State == "unmanaged" && !lock.baseline {
		return errors.New("baseline_required: existing control tables require verified baseline adoption")
	}
	if status.State == "incompatible" {
		return errors.New("version_incompatible: database version is not supported by this release")
	}
	if status.State == "current" || status.State == "pending" {
		expectedDigest, err := controlSchemaDigest(status.Current)
		if err != nil || status.AttemptDigest != expectedDigest {
			return errors.New("migration_release_mismatch: applied migration files differ from this release")
		}
		if err := checkControlSchema(ctx, conn, status.Current, false); err != nil {
			return err
		}
	}
	if lock.baseline {
		return lock.prepareBaseline(ctx, conn, status, digest)
	}
	bootstrap := false
	if status.State == "recovery_required" {
		if !lock.recover {
			return errors.New("recovery_required: inspect status and actual schema, then explicitly run recover")
		}
		if status.AttemptID == 0 {
			tables, err := controlTablesBeyondMigrationMetadata(ctx, conn)
			if err != nil || tables != 0 || status.Current != 0 {
				return errors.New("recovery_unverified: missing attempt with existing control state requires manual investigation")
			}
			bootstrap = true
		} else {
			if status.AttemptDigest != digest {
				return errors.New("recovery_release_mismatch: restore the original migration files before recovery")
			}
			var target int64
			if err := conn.QueryRowContext(ctx, `SELECT target_version FROM rcc_schema_migration_attempts WHERE id=? AND state='RUNNING'`, status.AttemptID).Scan(&target); err != nil || target != lock.version {
				return errors.New("recovery_unverified: attempt does not match the expected migration")
			}
			if err := checkControlSchema(ctx, conn, lock.version, status.Current < lock.version); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `UPDATE rcc_schema_migration_attempts SET recovery_count=recovery_count+1 WHERE id=?`, status.AttemptID); err != nil {
				return errors.New("recovery_unavailable: cannot record explicit recovery")
			}
			lock.attemptID = status.AttemptID
		}
	}
	if status.Current >= lock.version {
		return errSchemaAlreadyApplied
	}
	if lock.attemptID == 0 {
		if lock.recover && !bootstrap {
			return errors.New("recovery_not_required: use up for an ordinary upgrade")
		}
		if _, err := conn.ExecContext(ctx, schemaAttemptDDL); err != nil {
			return errors.New("migration_failed: cannot initialize attempt tracking; inspect status before retrying")
		}
		result, err := conn.ExecContext(ctx, `INSERT INTO rcc_schema_migration_attempts(target_version,release_digest,state) VALUES (?,?,'RUNNING')`, lock.version, digest)
		if err != nil {
			return errors.New("migration_failed: cannot record attempt; inspect status before retrying")
		}
		lock.attemptID, err = result.LastInsertId()
		if err != nil {
			return errors.New("migration_failed: attempt result unknown; inspect status before retrying")
		}
	}
	if status.Current == 0 {
		if err := initializeSchemaVersionZero(ctx, conn); err != nil {
			return errors.New("migration_failed: version table initialization incomplete; inspect status and explicitly recover")
		}
	}
	return nil
}

// MySQL may commit the ledger DDL before Goose inserts its zero row. Initialize
// that bookkeeping under the same lock and journal, before executing any control
// DDL. This never inserts a positive (success) migration version.
func initializeSchemaVersionZero(ctx context.Context, conn *sql.Conn) error {
	store, err := database.NewStore(database.DialectMySQL, schemaVersionTable)
	if err != nil {
		return err
	}
	var exists int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, schemaVersionTable).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		// Goose's MySQL ledger DDL omits ENGINE. Baseline version-prefix
		// registration relies on transactions regardless of the server default.
		if _, err := conn.ExecContext(ctx, `SET SESSION default_storage_engine='InnoDB'`); err != nil {
			return err
		}
		if err := store.CreateVersionTable(ctx, conn); err != nil {
			return err
		}
	}
	versions, err := store.ListMigrations(ctx, conn)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return store.Insert(ctx, conn, database.InsertRequest{Version: 0})
	}
	return nil
}

func (lock *schemaMigrationLock) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	var completionErr error
	if lock.attemptID > 0 {
		var applied int
		if err := conn.QueryRowContext(cleanup, `SELECT COUNT(*) FROM rcc_goose_db_version WHERE version_id=? AND is_applied=1`, lock.version).Scan(&applied); err == nil && applied == 1 {
			completionErr = checkControlSchema(cleanup, conn, lock.version, false)
			if completionErr == nil {
				state, previous := "SUCCEEDED", "RUNNING"
				if lock.baseline {
					state, previous = "BASELINED", "BASELINING"
				}
				if _, err := conn.ExecContext(cleanup, `UPDATE rcc_schema_migration_attempts SET state=?,finished_at=CURRENT_TIMESTAMP(6) WHERE id=? AND state=?`, state, lock.attemptID, previous); err != nil {
					completionErr = errors.New("migration_result_unknown: success could not be confirmed; inspect status and explicitly recover")
				}
			}
		}
	}
	var released sql.NullInt64
	err := conn.QueryRowContext(cleanup, `SELECT RELEASE_LOCK(CONCAT('rcc.schema:',LEFT(SHA2(DATABASE(),256),48)))`).Scan(&released)
	if err != nil || !released.Valid || released.Int64 != 1 {
		// Never return a connection holding a named lock to the pool.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return errors.New("migration_unlock_unknown: inspect migration status before retrying")
	}
	lock.failure = completionErr
	return completionErr
}

const schemaAttemptDDL = `CREATE TABLE IF NOT EXISTS rcc_schema_migration_attempts (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 target_version BIGINT NOT NULL,
 release_digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 recovery_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
 started_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 finished_at DATETIME(6) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`
