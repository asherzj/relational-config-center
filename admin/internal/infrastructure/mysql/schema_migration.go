package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql migrations/*_schema.json
var controlSchemaFiles embed.FS

const schemaVersionTable = "rcc_goose_db_version"

// SchemaMigrationStatus is the maintenance command's persisted migration progress.
type SchemaMigrationStatus struct {
	State          string  `json:"state"`
	Current        int64   `json:"current"`
	Required       int64   `json:"required"`
	Pending        []int64 `json:"pending"`
	AttemptID      int64   `json:"attempt_id,omitempty"`
	AttemptVersion int64   `json:"attempt_version,omitempty"`
	AttemptDigest  string  `json:"attempt_digest,omitempty"`
}

type SchemaMigrationOptions struct {
	LockTimeout time.Duration
	Recover     bool
}

func (adapter *Adapter) schemaProvider(options ...goose.ProviderOption) (*goose.Provider, error) {
	files, err := fs.Sub(controlSchemaFiles, "migrations")
	if err != nil {
		return nil, err
	}
	options = append(options, goose.WithTableName(schemaVersionTable), goose.WithDisableGlobalRegistry(true))
	return goose.NewProvider(goose.DialectMySQL, adapter.pool, files, options...)
}

type schemaQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// ControlSchemaStatus is deliberately read-only, including on uninitialized databases.
func (adapter *Adapter) ControlSchemaStatus(ctx context.Context) (SchemaMigrationStatus, error) {
	tx, err := adapter.pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return SchemaMigrationStatus{}, errors.New("status_unavailable: cannot open read-only migration snapshot")
	}
	defer tx.Rollback()
	return readControlSchemaStatus(ctx, tx)
}

func readControlSchemaStatus(ctx context.Context, db schemaQuerier) (SchemaMigrationStatus, error) {
	versions, err := controlSchemaVersions()
	if err != nil {
		return SchemaMigrationStatus{}, err
	}
	status := SchemaMigrationStatus{State: "uninitialized", Required: versions[len(versions)-1], Pending: versions}
	var versionTables, controlTables, attemptTables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_schema_migration_attempts'`).Scan(&attemptTables); err != nil {
		return status, errors.New("status_unavailable: cannot inspect migration attempts")
	}
	var attemptState string
	if attemptTables > 0 {
		err := db.QueryRowContext(ctx, `SELECT id,state,target_version,release_digest FROM rcc_schema_migration_attempts ORDER BY id DESC LIMIT 1`).Scan(&status.AttemptID, &attemptState, &status.AttemptVersion, &status.AttemptDigest)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return status, errors.New("status_unavailable: migration attempts cannot be read")
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, schemaVersionTable).Scan(&versionTables); err != nil {
		return status, errors.New("status_unavailable: check database connection and metadata permissions")
	}
	if versionTables == 0 {
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND LEFT(table_name,4)='rcc_'`).Scan(&controlTables); err != nil {
			return status, errors.New("status_unavailable: cannot inspect control tables")
		}
		if controlTables > 0 {
			status.State = "unmanaged"
		}
		if attemptTables > 0 {
			status.State = "recovery_required"
		}
		return status, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT version_id,is_applied FROM rcc_goose_db_version ORDER BY id`)
	if err != nil {
		return status, errors.New("status_unavailable: migration version table cannot be read")
	}
	index, invalid := 0, false
	for rows.Next() {
		var version int64
		var applied bool
		if err := rows.Scan(&version, &applied); err != nil {
			rows.Close()
			return status, errors.New("status_unavailable: migration history cannot be read")
		}
		want := int64(0)
		if index > 0 && index <= len(versions) {
			want = versions[index-1]
		}
		if index > len(versions) || version != want || !applied {
			invalid = true
		}
		status.Current = max(status.Current, version)
		index++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return status, errors.New("status_unavailable: migration history cannot be read")
	}
	status.State = "pending"
	status.Pending = []int64{}
	for _, version := range versions {
		if version > status.Current {
			status.Pending = append(status.Pending, version)
		}
	}
	if status.Current == status.Required {
		status.State, status.Pending = "current", []int64{}
	}
	if status.Current > status.Required || invalid || (attemptState == "SUCCEEDED" && (index == 0 || status.AttemptVersion != status.Current)) {
		status.State = "incompatible"
	}
	if status.State != "incompatible" && (index == 0 || attemptTables == 0 || attemptState != "SUCCEEDED") {
		status.State = "recovery_required"
	}
	return status, nil
}

func (adapter *Adapter) MigrateControlSchema(ctx context.Context, options SchemaMigrationOptions) (SchemaMigrationStatus, error) {
	status, err := adapter.ControlSchemaStatus(ctx)
	if err != nil {
		return status, err
	}
	locker := &schemaMigrationLock{wait: options.LockTimeout, recover: options.Recover}
	provider, err := adapter.schemaProvider(goose.WithSessionLocker(locker))
	if err != nil {
		return status, errors.New("migration_invalid: release migrations cannot be loaded")
	}
	// Up/UpTo call HasPending, which may create Goose's version table before
	// acquiring its session lock. ApplyVersion enters the locked path directly.
	for _, source := range provider.ListSources() {
		if options.Recover && source.Version < status.AttemptVersion {
			continue
		}
		locker.version = source.Version
		if _, err := provider.ApplyVersion(ctx, source.Version, true); err != nil {
			if errors.Is(err, errSchemaAlreadyApplied) {
				if options.Recover {
					break
				}
				continue
			}
			if locker.failure != nil {
				return status, locker.failure
			}
			return status, errors.New("migration_failed: inspect actual schema before retrying; DDL may have committed")
		}
		if options.Recover {
			break
		}
	}
	status, err = adapter.ControlSchemaStatus(ctx)
	if err == nil && status.State != "current" && !(options.Recover && status.State == "pending") {
		err = errors.New("migration_incomplete: inspect status and explicitly recover the unfinished attempt")
	}
	return status, err
}

func controlSchemaVersions() ([]int64, error) {
	entries, err := fs.ReadDir(controlSchemaFiles, "migrations")
	if err != nil {
		return nil, errors.New("migration_invalid: embedded release unavailable")
	}
	var versions []int64
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		number, _, ok := strings.Cut(entry.Name(), "_")
		version, err := strconv.ParseInt(number, 10, 64)
		if !ok || err != nil || version < 1 {
			return nil, errors.New("migration_invalid: invalid embedded version")
		}
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	if len(versions) == 0 {
		return nil, errors.New("migration_invalid: empty release")
	}
	for i := 1; i < len(versions); i++ {
		if versions[i] == versions[i-1] {
			return nil, errors.New("migration_invalid: duplicate embedded version")
		}
	}
	return versions, nil
}

// Bind recovery to the exact immutable SQL and schema manifests that started
// the attempt. A later release may append migrations without changing this prefix.
func controlSchemaDigest(version int64) (string, error) {
	entries, err := fs.ReadDir(controlSchemaFiles, "migrations")
	if err != nil {
		return "", errors.New("migration_invalid: embedded release unavailable")
	}
	digest := sha256.New()
	for _, entry := range entries {
		number, _, _ := strings.Cut(entry.Name(), "_")
		candidate, err := strconv.ParseInt(number, 10, 64)
		if err != nil || candidate > version {
			continue
		}
		data, err := controlSchemaFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return "", errors.New("migration_invalid: embedded release unavailable")
		}
		fmt.Fprintf(digest, "%d:%s%d:", len(entry.Name()), entry.Name(), len(data))
		digest.Write(data)
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}
