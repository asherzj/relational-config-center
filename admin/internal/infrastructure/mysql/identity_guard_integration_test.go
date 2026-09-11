//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	passwordadapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
	driver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPublicationJSONIdentityDoesNotUseURLPathSegments(t *testing.T) {
	ctx, adapter, _, _ := identityGuardDatabase(t)
	if err := adapter.gorm.Exec("CREATE TABLE guard_text_ids (id VARCHAR(64) PRIMARY KEY) ENGINE=InnoDB").Error; err != nil {
		t.Fatal(err)
	}
	publication, ctx := identityGuardApplication(t, ctx, adapter, "guard_text_ids")
	for _, id := range []string{".", "..", "键/值 ?#%", "%2F"} {
		t.Run(id, func(t *testing.T) {
			value := domain.JSONString(id)
			actual, err := publication.Add(ctx, "guard_text_ids", domain.MutationContent{"id": &value})
			if err != nil || actual != id {
				t.Fatalf("JSON identity %q: %q %v", id, actual, err)
			}
		})
	}
	assertIdentityGuardRows(t, adapter.pool, "guard_text_ids", 4)
}

func TestExplicitIdentityRejectsNontransactionalTableBeforeWriting(t *testing.T) {
	ctx, adapter, _, _ := identityGuardDatabase(t)
	if err := adapter.gorm.Exec("CREATE TABLE guard_myisam_ids (id DECIMAL(6,2) PRIMARY KEY) ENGINE=MyISAM").Error; err != nil {
		t.Fatal(err)
	}
	publication, ctx := identityGuardApplication(t, ctx, adapter, "guard_myisam_ids", "guard_myisam_auto_ids")
	_, err := publication.Add(ctx, "guard_myisam_ids", identityGuardContent("1.235"))
	if !errors.Is(err, application.ErrIncompatibleTable) {
		t.Errorf("ADD to MyISAM: got %v, want incompatible table", err)
	}
	assertIdentityGuardRows(t, adapter.pool, "guard_myisam_ids", 0)
	t.Run("generated identity also requires rollback support", func(t *testing.T) {
		if err := adapter.gorm.Exec("CREATE TABLE guard_myisam_auto_ids (id TINYINT(1) AUTO_INCREMENT PRIMARY KEY) ENGINE=MyISAM").Error; err != nil {
			t.Fatal(err)
		}
		mutation := publication
		_, err := mutation.Add(ctx, "guard_myisam_auto_ids", domain.MutationContent{})
		if !errors.Is(err, application.ErrIncompatibleTable) {
			t.Errorf("generated ADD to MyISAM: got %v, want incompatible table", err)
		}
		assertIdentityGuardRows(t, adapter.pool, "guard_myisam_auto_ids", 0)
	})
}

func TestExplicitIdentityReturnedAutoIDMustRemainSchemaAddressable(t *testing.T) {
	ctx, adapter, admin, _ := identityGuardDatabase(t)
	if _, err := admin.ExecContext(ctx, "CREATE TABLE guard_boolean_auto_ids (id TINYINT(1) AUTO_INCREMENT PRIMARY KEY, label VARCHAR(16)) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	mutation, ctx := identityGuardApplication(t, ctx, adapter, "guard_boolean_auto_ids")
	first, err := mutation.Add(ctx, "guard_boolean_auto_ids", domain.MutationContent{})
	if err != nil || first != "1" {
		t.Fatalf("first boolean auto identity: id=%q error=%v", first, err)
	}
	second, err := mutation.Add(ctx, "guard_boolean_auto_ids", domain.MutationContent{})
	var count int
	if readErr := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM guard_boolean_auto_ids").Scan(&count); readErr != nil {
		t.Fatal(readErr)
	}
	t.Logf("second boolean auto identity: id=%q error=%v, SQL rows=%d", second, err, count)
	if !errors.Is(err, application.ErrInvalidMutation) || count != 1 {
		t.Fatal("an auto-generated ID outside the public boolean contract must roll back")
	}
}

func TestExplicitIdentityKeepsEngineStableUntilInsertTransactionEnds(t *testing.T) {
	ctx, adapter, admin, _ := identityGuardDatabase(t)
	if _, err := admin.ExecContext(ctx, "CREATE TABLE guard_mdl_ids (id BIGINT PRIMARY KEY) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	publication, ctx := identityGuardApplication(t, ctx, adapter, "guard_mdl_ids")
	beforeInsert := make(chan struct{})
	resumeInsert := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(resumeInsert) }) }
	t.Cleanup(release)
	var insertDriverError error
	if err := adapter.gorm.Callback().Create().After("gorm:create").Register("test:observe_identity_insert", func(tx *gorm.DB) {
		if tx.Statement.Table != "guard_mdl_ids" {
			return
		}
		insertDriverError = tx.Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.gorm.Callback().Create().Before("gorm:create").Register("test:hold_identity_insert", func(tx *gorm.DB) {
		if tx.Statement.Table != "guard_mdl_ids" {
			return
		}
		close(beforeInsert)
		select {
		case <-resumeInsert:
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatal(err)
	}
	inserted := make(chan error, 1)
	go func() {
		_, err := publication.Add(ctx, "guard_mdl_ids", identityGuardContent("77"))
		inserted <- err
	}()
	select {
	case <-beforeInsert:
	case err := <-inserted:
		t.Fatalf("INSERT did not reach the pre-execution barrier: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	ddlCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	altered := make(chan error, 1)
	go func() {
		_, err := admin.ExecContext(ddlCtx, "ALTER TABLE guard_mdl_ids ENGINE=MyISAM")
		altered <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	locked := false
	for time.Now().Before(deadline) {
		var pending int
		if err := admin.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.metadata_locks
WHERE OBJECT_SCHEMA='identity_guard' AND OBJECT_NAME='guard_mdl_ids' AND LOCK_STATUS='PENDING'`).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if pending > 0 {
			locked = true
			break
		}
		select {
		case err := <-altered:
			t.Fatalf("ALTER ENGINE completed before INSERT began: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !locked {
		t.Fatal("ALTER ENGINE did not wait for the pre-INSERT metadata lock")
	}
	release()
	rows := 1
	if err := <-inserted; err != nil {
		// A queued exclusive DDL lock can deadlock the INSERT's metadata-lock
		// upgrade. That is a safe failure only if the whole row write rolls back.
		var mysqlError *driver.MySQLError
		if !errors.Is(err, application.ErrMutationUnavailable) || !errors.As(insertDriverError, &mysqlError) || mysqlError.Number != 1213 {
			t.Fatalf("INSERT after metadata-lock probe: %v (driver: %v)", err, insertDriverError)
		}
		t.Logf("concurrent DDL caused real MySQL %d; row transaction must roll back", mysqlError.Number)
		rows = 0
	}
	if err := <-altered; err != nil {
		t.Fatalf("ALTER ENGINE after commit: %v", err)
	}
	var engine string
	if err := admin.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='identity_guard' AND TABLE_NAME='guard_mdl_ids'").Scan(&engine); err != nil {
		t.Fatal(err)
	}
	if engine != "MyISAM" {
		t.Fatalf("engine after lock release: %s", engine)
	}
	assertIdentityGuardRows(t, admin, "guard_mdl_ids", rows)
}

func TestExplicitIdentityReadPermissionFailureAfterInsertRollsBack(t *testing.T) {
	ctx, _, admin, settings := identityGuardDatabase(t)
	for _, statement := range []string{
		"CREATE TABLE guard_permission_ids (id BIGINT PRIMARY KEY) ENGINE=InnoDB",
		"CREATE USER 'identity_writer'@'%' IDENTIFIED BY 'isolated-identity-writer'",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON identity_guard.guard_permission_ids TO 'identity_writer'@'%'",
		"GRANT TRIGGER ON identity_guard.* TO 'identity_writer'@'%'",
		"GRANT PROCESS ON *.* TO 'identity_writer'@'%'",
	} {
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := admin.QueryContext(ctx, "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA='identity_guard' AND TABLE_NAME LIKE 'rcc\\_%'")
	if err != nil {
		t.Fatal(err)
	}
	var controls []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		controls = append(controls, table)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range controls {
		if _, err := admin.ExecContext(ctx, "GRANT SELECT, INSERT, UPDATE, DELETE ON identity_guard.`"+table+"` TO 'identity_writer'@'%'"); err != nil {
			t.Fatal(err)
		}
	}
	limitedSettings := settings.Clone()
	limitedSettings.User, limitedSettings.Passwd = "identity_writer", "isolated-identity-writer"
	adapter := identityGuardAdapter(t, limitedSettings)
	publication, ctx := identityGuardApplication(t, ctx, adapter, "guard_permission_ids")
	var callbackErr error
	var insertedRows int
	var readDenied bool
	if err := adapter.gorm.Callback().Create().After("gorm:create").Register("test:revoke_identity_read", func(tx *gorm.DB) {
		if tx.Statement.Table != "guard_permission_ids" {
			return
		}
		if tx.Error != nil {
			callbackErr = tx.Error
			return
		}
		callbackErr = tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT COUNT(*) FROM guard_permission_ids WHERE id=88").Row().Scan(&insertedRows)
		if callbackErr == nil {
			_, callbackErr = admin.ExecContext(ctx, "REVOKE SELECT ON identity_guard.guard_permission_ids FROM 'identity_writer'@'%'")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.gorm.Callback().Row().After("gorm:row").Register("test:observe_identity_read_error", func(tx *gorm.DB) {
		var mysqlError *driver.MySQLError
		if errors.As(tx.Error, &mysqlError) && mysqlError.Number == 1142 {
			readDenied = true
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = publication.Add(ctx, "guard_permission_ids", identityGuardContent("88"))
	if callbackErr != nil {
		t.Fatalf("real post-INSERT permission change: %v", callbackErr)
	}
	if insertedRows != 1 || !readDenied {
		t.Fatalf("expected one transaction-local row before real SELECT 1142; rows=%d denied=%v", insertedRows, readDenied)
	}
	if !errors.Is(err, application.ErrMutationUnavailable) {
		t.Fatalf("post-INSERT SELECT failure: got %v, want unavailable", err)
	}
	assertIdentityGuardRows(t, admin, "guard_permission_ids", 0)
}

func TestExplicitIdentityNeverReturnsExistingRowAfterTriggerChangesKey(t *testing.T) {
	ctx, adapter, admin, _ := identityGuardDatabase(t)
	for _, statement := range []string{
		"CREATE TABLE guard_trigger_ids (id BIGINT PRIMARY KEY, label VARCHAR(16) NOT NULL DEFAULT 'new') ENGINE=InnoDB",
		"INSERT INTO guard_trigger_ids (id, label) VALUES (77, 'old')",
		"CREATE TRIGGER guard_rewrite_identity BEFORE INSERT ON guard_trigger_ids FOR EACH ROW SET NEW.id=88",
	} {
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	publication, ctx := identityGuardApplication(t, ctx, adapter, "guard_trigger_ids")
	id, err := publication.Add(ctx, "guard_trigger_ids", identityGuardContent("77"))
	var oldRows, newRows int
	if readErr := admin.QueryRowContext(ctx, "SELECT COUNT(CASE WHEN id=77 AND label='old' THEN 1 END), COUNT(CASE WHEN id=88 AND label='new' THEN 1 END) FROM guard_trigger_ids").Scan(&oldRows, &newRows); readErr != nil {
		t.Fatal(readErr)
	}
	t.Logf("submitted ID=77; returned ID=%q error=%v; original 77 rows=%d, new 88 rows=%d", id, err, oldRows, newRows)
	if !errors.Is(err, application.ErrDuplicateKey) || oldRows != 1 || newRows != 0 {
		t.Fatal("a trigger must not turn an existing submitted key into a successful identity; its new row must roll back")
	}
	if _, err := admin.ExecContext(ctx, "DELETE FROM guard_trigger_ids"); err != nil {
		t.Fatal(err)
	}
	_, err = publication.Add(ctx, "guard_trigger_ids", identityGuardContent("77"))
	if !errors.Is(err, application.ErrPublicationUnsupported) {
		t.Fatalf("trigger changed an unused key: got %v, want invalid mutation", err)
	}
	assertIdentityGuardRows(t, admin, "guard_trigger_ids", 0)
}

func TestPublicationRejectsIdentityCreatedAfterApproval(t *testing.T) {
	ctx, adapter, owner, _ := identityGuardDatabase(t)
	if _, err := owner.ExecContext(ctx, "CREATE TABLE guard_current_ids (id BIGINT PRIMARY KEY) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	publication, ctx := identityGuardApplication(t, ctx, adapter, "guard_current_ids")
	approved, key, err := publication.approve(ctx, "guard_current_ids", identityGuardContent("77"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ExecContext(ctx, "INSERT INTO guard_current_ids(id) VALUES(77)"); err != nil {
		t.Fatal(err)
	}
	_, err = publication.orders.Execute(ctx, approved.ID, application.SubmitReleaseInput{ExpectedVersion: approved.Version}, key+"-execute")
	if !errors.Is(err, application.ErrRecordVersionConflict) && !errors.Is(err, application.ErrDuplicateKey) {
		t.Fatalf("new current identity: %v", err)
	}
	assertIdentityGuardRows(t, owner, "guard_current_ids", 1)
	stored, err := adapter.ReadReleaseHeader(ctx, approved.ID)
	if err != nil || stored.State != "APPROVED" || len(stored.Executions) != 0 {
		t.Fatalf("failed approved publication: %#v %v", stored, err)
	}
}

func TestExplicitIdentityAutoResultBelongsToThisInsert(t *testing.T) {
	ctx, adapter, admin, _ := identityGuardDatabase(t)
	adapter.pool.SetMaxOpenConns(1)
	type identityCase struct {
		name, supplied, trigger, start, mode, wantID string
		seeded, unsigned, wantInvalid                bool
	}
	cases := []identityCase{}
	for _, seeded := range []bool{false, true} {
		for _, mode := range []string{"omitted", "zero", "explicit77"} {
			test := identityCase{name: fmt.Sprintf("%s_%t", mode, seeded), trigger: "88", seeded: seeded, wantInvalid: true}
			if mode == "zero" {
				test.supplied = "0"
			} else if mode == "explicit77" {
				test.supplied, test.wantID, test.wantInvalid = "77", "", true
			}
			cases = append(cases, test)
		}
	}
	cases = append(cases,
		identityCase{name: "unsigned_generated", unsigned: true, start: "18446744073709551614", wantID: "18446744073709551614"},
		identityCase{name: "unsigned_trigger", unsigned: true, trigger: "18446744073709551614", wantInvalid: true},
		identityCase{name: "no_auto_zero", unsigned: true, supplied: "0", mode: "NO_AUTO_VALUE_ON_ZERO", wantID: "0"},
	)
	tables := make([]string, 0, len(cases))
	for _, test := range cases {
		table := "guard_result_" + test.name
		tables = append(tables, table)
		kind := "BIGINT"
		if test.unsigned {
			kind += " UNSIGNED"
		}
		if _, err := admin.ExecContext(ctx, "CREATE TABLE "+table+" (id "+kind+" AUTO_INCREMENT PRIMARY KEY, label VARCHAR(16) NOT NULL DEFAULT 'new') ENGINE=InnoDB"); err != nil {
			t.Fatal(err)
		}
		if test.start != "" {
			if _, err := admin.ExecContext(ctx, "ALTER TABLE "+table+" AUTO_INCREMENT="+test.start); err != nil {
				t.Fatal(err)
			}
		}
		if test.seeded {
			if _, err := admin.ExecContext(ctx, "INSERT INTO "+table+" (id,label) VALUES (77,'old')"); err != nil {
				t.Fatal(err)
			}
		}
		if test.trigger != "" {
			if _, err := admin.ExecContext(ctx, "CREATE TRIGGER "+table+"_rewrite BEFORE INSERT ON "+table+" FOR EACH ROW SET NEW.id="+test.trigger); err != nil {
				t.Fatal(err)
			}
		}
	}
	mutation, ctx := identityGuardApplication(t, ctx, adapter, tables...)
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := adapter.gorm.Exec("SET SESSION sql_mode=?", test.mode).Error; err != nil {
				t.Fatal(err)
			}
			// The pool has one connection, so the ADD inherits this deliberately
			// stale session value. Only its own INSERT result can identify the row.
			var previous int
			if err := adapter.gorm.Raw("SELECT LAST_INSERT_ID(6)").Scan(&previous).Error; err != nil {
				t.Fatal(err)
			}
			content := domain.MutationContent{}
			if test.supplied != "" {
				id := domain.JSONString(test.supplied)
				content["id"] = &id
			}
			table := "guard_result_" + test.name
			id, err := mutation.Add(ctx, table, content)
			if test.wantInvalid {
				if !errors.Is(err, application.ErrPublicationUnsupported) && !errors.Is(err, application.ErrReleaseAutoIDAmbiguous) && !(test.seeded && test.supplied == "77" && errors.Is(err, application.ErrDuplicateKey)) {
					t.Errorf("rewritten explicit identity: id=%q error=%v", id, err)
				}
			} else if err != nil || id != test.wantID {
				t.Errorf("returned ID must belong to this INSERT: id=%q error=%v want=%q", id, err, test.wantID)
			}
			var oldRows, newRows int
			var storedID sql.NullString
			if err := admin.QueryRowContext(ctx, "SELECT COUNT(CASE WHEN label='old' THEN 1 END), COUNT(CASE WHEN label='new' THEN 1 END), MAX(CASE WHEN label='new' THEN CAST(id AS CHAR) END) FROM "+table).Scan(&oldRows, &newRows, &storedID); err != nil {
				t.Fatal(err)
			}
			wantOld, wantNew := 0, 1
			if test.seeded {
				wantOld = 1
			}
			if test.wantInvalid {
				wantNew = 0
			}
			if oldRows != wantOld || newRows != wantNew || (!test.wantInvalid && (!storedID.Valid || storedID.String != test.wantID)) {
				t.Fatalf("SQL old=%d new=%d newID=%v, expected old=%d new=%d id=%q", oldRows, newRows, storedID, wantOld, wantNew, test.wantID)
			}
		})
	}
}

func identityGuardContent(id string) domain.MutationContent {
	value := domain.JSONString(id)
	return domain.MutationContent{"id": &value}
}

func identityGuardDatabase(t *testing.T) (context.Context, *Adapter, *sql.DB, *driver.Config) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	container, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithDatabase("identity_guard"), tcmysql.WithUsername("root"), tcmysql.WithPassword("isolated-identity-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})
	settings, err := driver.ParseDSN(container.MustConnectionString(ctx, "parseTime=true"))
	if err != nil {
		t.Fatal(err)
	}
	settings.ClientFoundRows = true
	adapter := identityGuardAdapter(t, settings)
	if _, err := adapter.MigrateControlSchema(ctx, SchemaMigrationOptions{LockTimeout: 5 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("mysql", settings.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	return ctx, adapter, admin, settings
}

func identityGuardApplication(t *testing.T, ctx context.Context, adapter *Adapter, tables ...string) (*identityGuardPublication, context.Context) {
	t.Helper()
	for _, statement := range []string{
		"INSERT INTO rcc_query_policies(code,name,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,status,creator,modifier) VALUES ('guard_query_v1','Guard query','page_query','id','ASC',5,20,'ACTIVE','test','test')",
		"INSERT INTO rcc_mutation_policies(code,name,type_code,allow_add,allow_modify,allow_delete,status,creator,modifier) VALUES ('guard_mutation_v1','Guard mutation','single_table_mutation',1,0,0,'ACTIVE','test','test')",
	} {
		if err := adapter.gorm.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range tables {
		if err := adapter.gorm.Exec("INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES (?,'guard_query_v1','guard_mutation_v1',1,'test','test')", table).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Actual registration, explicit maintenance grants and separate Login Sessions
	// authorize the same ReleaseOrders API used by HTTP. No raw business-write facade.
	auth := application.NewAuthentication(adapter, passwordadapter.NewArgon2id(), nil, adapter, application.AuthenticationLimits{})
	actor := func(name string) context.Context {
		preauth, csrf, err := auth.Prepare(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := auth.CheckPreauth(ctx, preauth, csrf); err != nil {
			t.Fatal(err)
		}
		registered, err := auth.Register(ctx, application.Registration{Username: name, Email: name + "@example.com", Password: "correct horse battery staple"}, preauth, "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = application.NewAccountMaintenance(adapter, passwordadapter.NewArgon2id()).GrantAdmin(ctx, application.AccountSelector{Username: name})
		if err != nil {
			t.Fatal(err)
		}
		operator, err := auth.AuthenticateRequest(ctx, registered.Token, registered.CSRF, true)
		if err != nil {
			t.Fatal(err)
		}
		return operator.Bind(ctx)
	}
	return &identityGuardPublication{orders: application.NewReleaseOrders(adapter), reviewer: actor("identity.reviewer")}, actor("identity.applicant")
}

type identityGuardPublication struct {
	orders   *application.ReleaseOrders
	reviewer context.Context
	sequence uint64
}

func (p *identityGuardPublication) approve(ctx context.Context, table string, content domain.MutationContent) (domain.ReleaseOrder, string, error) {
	p.sequence++
	key := fmt.Sprintf("identity-guard-%d", p.sequence)
	order, err := p.orders.Create(ctx, application.DraftInput{Title: "身份保护测试发布单", Items: []application.DraftItemInput{{TableName: table, Operation: "ADD", Content: content}}}, key+"-create")
	if err != nil {
		return order, key, err
	}
	order, err = p.orders.Submit(ctx, order.ID, application.SubmitReleaseOrderInput{ExpectedVersion: order.Version}, key+"-submit")
	if err != nil {
		return order, key, err
	}
	// The separate administrator reads the pending fallback scope before confirming it.
	review, err := p.orders.Get(p.reviewer, order.ID)
	if err != nil {
		return order, key, err
	}
	order, err = p.orders.Approve(p.reviewer, order.ID, application.ReleaseDecisionInput{
		ExpectedVersion:          review.Version,
		Reason:                   "independent identity review",
		ConfirmedTables:          review.ApprovalContext.ApprovableTables,
		ExpectedApprovalRevision: review.ApprovalContext.Revision,
	}, key+"-approve")
	return order, key, err
}
func (p *identityGuardPublication) Add(ctx context.Context, table string, content domain.MutationContent) (string, error) {
	order, key, err := p.approve(ctx, table, content)
	if err != nil {
		return "", err
	}
	order, err = p.orders.Execute(ctx, order.ID, application.SubmitReleaseInput{ExpectedVersion: order.Version}, key+"-execute")
	if err != nil {
		return "", err
	}
	return order.Items[0].Publication.ID, nil
}

func identityGuardAdapter(t *testing.T, settings *driver.Config) *Adapter {
	t.Helper()
	database, err := gorm.Open(gormmysql.New(gormmysql.Config{DSN: settings.FormatDSN(), SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return &Adapter{database: settings.DBName, gorm: database, pool: pool}
}

func assertIdentityGuardRows(t *testing.T, database *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow("SELECT COUNT(*) FROM `" + table + "`").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("%s has %d rows, want %d", table, got, want)
	}
}
