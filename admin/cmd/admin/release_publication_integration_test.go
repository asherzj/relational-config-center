//go:build integration

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	mysqldriver "github.com/go-sql-driver/mysql"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func approvePublication(t *testing.T, app *adminApplication, reviewer *httptest.ResponseRecorder, input, key string) string {
	t.Helper()
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", input, key+"-create")
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var order struct{ ID string }
	if err := json.Unmarshal(created.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/release-orders/" + order.ID
	for _, step := range []struct{ action, body string }{{"submit", `{"expected_version":"1"}`}, {"approve", `{"expected_version":"2","reason":"checked intent"}`}} {
		var response *httptest.ResponseRecorder
		if step.action == "approve" {
			response = releaseActorRequest(t, app, reviewer, "POST", path+"/"+step.action, step.body, key+"-"+step.action)
		} else {
			response = releaseRequest(t, app, "POST", path+"/"+step.action, step.body, key+"-"+step.action)
		}
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", step.action, response.Code, response.Body)
		}
	}
	return path
}

// AC-026/027: actual publication captures the database defaults and generated row.
func TestReleasePublicationAddsFinalRow(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	reviewer := registerAccount(t, app, "publication.reviewer", "publication.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER","PUBLISHER"]`, "1", "publication-roles")
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"published","label":"","metadata":"null"}}]}`, "publication-add")
	response := releaseActorRequest(t, app, reviewer, "POST", path+"/execute", `{"expected_version":"3"}`, "publication-execute")
	if response.Code != 200 {
		t.Fatalf("execute: %d %s", response.Code, response.Body)
	}
	var result struct {
		State, Version string
		Publication    struct {
			TableVersion string `json:"table_version"`
			Commands     []struct {
				ID            string
				RecordVersion string `json:"record_version"`
				Final         struct {
					Deleted bool
					Fields  []struct {
						Name, Encoding string
						Value          *string
					}
				}
			}
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "SUCCEEDED" || result.Version != "4" || result.Publication.TableVersion != "1" || len(result.Publication.Commands) != 1 {
		t.Fatal(response.Body)
	}
	command := result.Publication.Commands[0]
	row, version := recordVersionRow(t, app, "mutation_add_items", command.ID)
	if version != "1" || command.RecordVersion != "1" || *row["defaulted_value"] != "database-default" || *row["generated_value"] != "published:generated" || *row["label"] != "" || *row["metadata"] != "null" || row["nullable_value"] != nil {
		t.Fatalf("final row %v version %s: %s", row, version, response.Body)
	}
	fields := map[string]struct {
		encoding string
		value    *string
	}{}
	for _, f := range command.Final.Fields {
		fields[f.Name] = struct {
			encoding string
			value    *string
		}{f.Encoding, f.Value}
	}
	if len(fields) != len(row) || fields["nullable_value"].encoding != "sql_null" || fields["metadata"].encoding != "json" || fields["metadata"].value == nil || *fields["metadata"].value != "null" {
		t.Fatal(response.Body)
	}
	replay := releaseActorRequest(t, app, reviewer, "POST", path+"/execute", `{"expected_version":"3"}`, "publication-execute")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatalf("replay: %d %s", replay.Code, replay.Body)
	}
}

// AC-035: even a hidden cross-schema cascade must be rejected before DML.
func TestPublicationRejectsUntrackedCascade(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	for _, statement := range []string{`GRANT PROCESS ON *.* TO 'rcc_admin'@'%'`, `CREATE DATABASE hidden_child`, `CREATE TABLE hidden_child.child(id bigint PRIMARY KEY,parent_id bigint unsigned,FOREIGN KEY(parent_id) REFERENCES rcc_test.mutation_delete_parents(id) ON DELETE CASCADE) ENGINE=InnoDB`, `INSERT INTO hidden_child.child VALUES(1,1)`, `DELETE FROM mutation_delete_children`} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowDelete: true})
	reviewer := registerAccount(t, app, "cascade.reviewer", "cascade.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "cascade-roles")
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_delete_parents","items":[{"operation":"DELETE","id":"1","expected_record_version":"0","content":{}}]}`, "cascade")
	response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "cascade-execute")
	assertIntegrationErrorCode(t, response, 422, "publication_unsupported")
	row, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if version != "0" || *row["code"] != "delete-rollback" {
		t.Fatal("untracked write escaped")
	}
	var count int
	if err := owner.QueryRow(`SELECT COUNT(*) FROM hidden_child.child`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("hidden cascade count %d err %v", count, err)
	}
}

func TestPublicationSupportsTargetRowTrigger(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	if _, err := owner.Exec(`CREATE TRIGGER final_label BEFORE INSERT ON mutation_add_items FOR EACH ROW SET NEW.label=CONCAT(NEW.label,'!')`); err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	reviewer := registerAccount(t, app, "trigger.reviewer", "trigger.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "trigger-roles")
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"triggered","label":"intent"}}]}`, "trigger")
	response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "trigger-execute")
	if response.Code != 200 {
		t.Fatalf("target trigger: %d %s", response.Code, response.Body)
	}
	var finalLabel string
	if err := owner.QueryRow(`SELECT label FROM mutation_add_items WHERE code='triggered'`).Scan(&finalLabel); err != nil || finalLabel != "intent!" {
		t.Fatalf("final %q err %v", finalLabel, err)
	}
	var order struct {
		Publication struct {
			Commands []struct {
				Final struct {
					Fields []struct {
						Name  string
						Value *string
					}
				}
			}
		}
	}
	json.Unmarshal(response.Body.Bytes(), &order)
	for _, f := range order.Publication.Commands[0].Final.Fields {
		if f.Name == "label" && f.Value != nil && *f.Value == "intent!" {
			return
		}
	}
	t.Fatal("trigger final value missing from command")
}

// AC-030: each control-data failure rolls back the complete publication. The
// injected failures are real MySQL trigger errors at the persistence boundary.
func TestPublicationAtomicPersistenceFailures(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	deliveryExec(t, owner, `CREATE TABLE z_atomic_second(id INT PRIMARY KEY,label VARCHAR(80))`)
	deliveryExec(t, owner, `INSERT INTO z_atomic_second VALUES(1,'before')`)
	enableMutationPolicy(t, app, "z_atomic_second", mutationPolicyFixture{AllowModify: true})
	reviewer := registerAccount(t, app, "atomic.reviewer", "atomic.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "atomic-roles")
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"committed"}},{"operation":"ADD","content":{"code":"new-atomic"}},{"table_name":"z_atomic_second","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"label":"after"}}]}`, "atomic")
	for _, failure := range []struct{ table, event, condition string }{{"rcc_record_versions", "INSERT", "TRUE"}, {"rcc_publication_commands", "INSERT", "TRUE"}, {"rcc_table_publications", "UPDATE", "NEW.table_version>0 AND NEW.table_name='z_atomic_second'"}, {"rcc_refresh_notifications", "INSERT", "NEW.table_name='z_atomic_second'"}, {"rcc_release_targets", "INSERT", "TRUE"}, {"rcc_release_orders", "UPDATE", "NEW.state='SUCCEEDED'"}, {"rcc_release_requests", "UPDATE", "NEW.result IS NOT NULL"}} {
		t.Run(failure.table, func(t *testing.T) {
			statement := fmt.Sprintf("CREATE TRIGGER fail_publication BEFORE %s ON %s FOR EACH ROW BEGIN IF %s THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected persistence failure'; END IF; END", failure.event, failure.table, failure.condition)
			if _, err := owner.Exec(statement); err != nil {
				t.Fatal(err)
			}
			response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "atomic-execute")
			if response.Code != 503 {
				t.Fatalf("failure must reject: %d %s", response.Code, response.Body)
			}
			if _, err := owner.Exec(`DROP TRIGGER fail_publication`); err != nil {
				t.Fatal(err)
			}
			row, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
			if version != "0" || *row["code"] != "delete-rollback" {
				t.Fatal("partial configuration or record version")
			}
			second, secondVersion := recordVersionRow(t, app, "z_atomic_second", "1")
			if secondVersion != "0" || *second["label"] != "before" {
				t.Fatal("partial second table value/version")
			}
			batchEdgeCounts(t, owner, map[string]int{`SELECT COUNT(*) FROM rcc_release_details WHERE publication IS NOT NULL`: 0, `SELECT COUNT(*) FROM rcc_release_executions`: 0, `SELECT COUNT(*) FROM rcc_release_table_references`: 2})
			current := releaseRequest(t, app, "GET", path, "", "")
			if !strings.Contains(current.Body.String(), `"state":"APPROVED"`) || strings.Contains(current.Body.String(), `"action":"EXECUTE"`) {
				t.Fatal(current.Body)
			}
			var targets, commands, notifications, requests, versions int
			for query, dest := range map[string]*int{`SELECT COUNT(*) FROM rcc_release_targets`: &targets, `SELECT COUNT(*) FROM rcc_publication_commands`: &commands, `SELECT COUNT(*) FROM rcc_refresh_notifications`: &notifications, `SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'execute:%' AND result IS NOT NULL`: &requests, `SELECT COUNT(*) FROM rcc_table_publications`: &versions} {
				if err := owner.QueryRow(query).Scan(dest); err != nil {
					t.Fatal(err)
				}
			}
			if targets != 2 || commands != 0 || notifications != 0 || requests != 0 || versions != 0 {
				t.Fatalf("partial state: targets %d commands %d notifications %d requests %d table versions %d", targets, commands, notifications, requests, versions)
			}
		})
	}
	response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "atomic-execute")
	if response.Code != 200 {
		t.Fatalf("same key retry: %d %s", response.Code, response.Body)
	}
	batchEdgeCounts(t, owner, map[string]int{`SELECT COUNT(*) FROM rcc_refresh_notifications`: 2, `SELECT COUNT(*) FROM rcc_table_publications WHERE table_version=1`: 2, `SELECT COUNT(*) FROM z_atomic_second WHERE label='after'`: 1})
}

func TestOldRecordWriteRoutesAreRemoved(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	for _, request := range []struct{ method, path, body string }{{"POST", "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"bypass","label":"bypass"}}`}, {"PATCH", "/api/v1/tables/mutation_add_items/rows/1", `{"expected_version":"0","content":{"label":"bypass"}}`}, {"DELETE", "/api/v1/tables/mutation_add_items/rows/1", `{"expected_version":"0"}`}} {
		response := releaseRequest(t, app, request.method, request.path, request.body, "old-route")
		assertIntegrationErrorCode(t, response, 404, "route_not_found")
	}
}

func TestPublicationIdentityCanonicalAndDelete(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	ownerDriver := *driver
	ownerDriver.User = "root"
	db := deliveryDB(t, &ownerDriver)
	deliveryExec(t, db, `ALTER TABLE mutation_add_items AUTO_INCREMENT=9223372036854775808`)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	add := publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"code":"large-id","label":"lossless"}}`)
	command := publishedFixtureCommand(t, add)
	if command.ID != "9223372036854775808" || command.RecordVersion != "1" || !command.Before.Deleted {
		t.Fatalf("large auto id: %+v", command)
	}
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	empty := publicationFixtureRequest(t, app, "ADD", "mutation_supplied_id_items", "", `{"content":{"id":"","label":"before"}}`)
	first := publishedFixtureCommand(t, empty)
	if first.ID != "" {
		t.Fatal("empty ID changed")
	}
	modify := publicationFixtureRequest(t, app, "MODIFY", "mutation_supplied_id_items", "", `{"expected_version":"1","content":{"label":"after"}}`)
	second := publishedFixtureCommand(t, modify)
	if second.ID != "" || second.RecordVersion != "2" {
		t.Fatal(modify.Body)
	}
	if second.Before.Verify(second.Final.SchemaDigest) != nil || second.Before.Fields[1].Value == nil || *second.Before.Fields[1].Value != "before" || *second.Final.Fields[1].Value != "after" {
		t.Fatal("before/final lost")
	}
	deletion := publicationFixtureRequest(t, app, "DELETE", "mutation_supplied_id_items", "", `{"expected_version":"2"}`)
	third := publishedFixtureCommand(t, deletion)
	if third.ID != "" || third.RecordVersion != "3" || third.Sequence != "3" || len(third.Final.Fields) != 0 || *third.Before.Fields[1].Value != "after" {
		t.Fatal(deletion.Body)
	}
	recreate := publicationFixtureRequest(t, app, "ADD", "mutation_supplied_id_items", "", `{"content":{"id":"","label":"new"}}`)
	fourth := publishedFixtureCommand(t, recreate)
	if fourth.RecordVersion != "4" || fourth.TableVersion != "4" {
		t.Fatal(recreate.Body)
	}
}

func TestPublicationRejectsImplicitWritesAndAuditSpoofing(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	ownerDriver := *driver
	ownerDriver.User = "root"
	db := deliveryDB(t, &ownerDriver)
	deliveryExec(t, db, `CREATE TABLE trigger_side_effect(id int PRIMARY KEY)`)
	deliveryExec(t, db, `ALTER TABLE mutation_add_items ADD creator varchar(64), ADD executed_at datetime(6)`)
	settings := integrationConfig(driver)
	settings.MySQL.MaxIdleConnections = 0
	app, err := newApplication(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	creator, stamp := "creator", "executed_at"
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, CreateOperatorField: &creator, CreateTimeField: &stamp})
	reviewer := publicationFixtureReviewer(t, app)
	for i, body := range []string{`INSERT INTO trigger_side_effect VALUES(1)`, `SET NEW.id=999`, `SET NEW.creator='spoofed'`, `SET NEW.executed_at='2000-01-01'`, `BEGIN SET NEW.label='changed'; INSERT INTO trigger_side_effect VALUES(1); END`, `SET @publication_side_effect=1`} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			deliveryExec(t, db, "CREATE TRIGGER hidden_write BEFORE INSERT ON mutation_add_items FOR EACH ROW "+body)
			path := approvePublication(t, app, reviewer, fmt.Sprintf(`{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"unsupported-%d","label":"intent"}}]}`, i), fmt.Sprintf("unsupported-%d", i))
			response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, fmt.Sprintf("unsupported-execute-%d", i))
			assertIntegrationErrorCode(t, response, 422, "publication_unsupported")
			var rows, effects int
			db.QueryRow(`SELECT COUNT(*) FROM mutation_add_items`).Scan(&rows)
			db.QueryRow(`SELECT COUNT(*) FROM trigger_side_effect`).Scan(&effects)
			if rows != 0 || effects != 0 {
				t.Fatal("unsupported trigger ran")
			}
			deliveryExec(t, db, `DROP TRIGGER hidden_write`)
		})
	}
	// PROCESS is required for visibility of hidden cross-schema FK metadata.
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"permissions","label":"intent"}}]}`, "permissions")
	deliveryExec(t, db, `REVOKE PROCESS ON *.* FROM 'rcc_admin'@'%'`)
	// This fixture has no idle connections, so global privilege changes apply.
	denied := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "permissions-execute")
	assertIntegrationErrorCode(t, denied, 422, "publication_metadata_permission")
	deliveryExec(t, db, `GRANT PROCESS ON *.* TO 'rcc_admin'@'%'`)
	success := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "permissions-execute")
	publishedFixtureCommand(t, success)
}

func TestPublicationPublisherHistoryAndApprovalSurvivesRevocation(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	creator, stamp := "creator", "occurred_at"
	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{AllowAdd: true, CreateOperatorField: &creator, CreateTimeField: &stamp})
	reviewer := registerAccount(t, app, "history.reviewer", "history.reviewer@example.com", "correct horse battery staple")
	publisher := registerAccount(t, app, "history.publisher", "history.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "history-reviewer-role")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "history-publisher-role")
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_auto_fill_items","items":[{"operation":"ADD","content":{"code":"history","status":"active","quantity":"1"}}]}`, "history")
	grantReleaseRole(t, app, reviewer, `["VIEWER"]`, "2", "history-reviewer-revoked")
	denied := releaseActorRequest(t, app, reviewer, "POST", path+"/execute", `{"expected_version":"3"}`, "revoked-execute")
	assertIntegrationErrorCode(t, denied, 403, "permission_denied")
	response := releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "history-execute")
	command := publishedFixtureCommand(t, response)
	var order domain.ReleaseOrder
	json.Unmarshal(response.Body.Bytes(), &order)
	row, _ := recordVersionRow(t, app, "mutation_auto_fill_items", command.ID)
	c := accountID(t, publisher)
	b := accountID(t, reviewer)
	a := integrationAccountID(t, app)
	if *row["creator"] != c || order.Publication.PublisherID != c {
		t.Fatal("publisher permanent identity lost")
	}
	executed, err := time.Parse(time.RFC3339Nano, order.Publication.ExecutedAt)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := time.Parse("2006-01-02 15:04:05.999999", *row["occurred_at"])
	if err != nil || actual.Sub(executed) > time.Microsecond || executed.Sub(actual) > time.Microsecond {
		t.Fatalf("not execution database time: %v %v %v", actual, executed, err)
	}
	expected := []string{a, a, b, c}
	if len(order.History) != 4 {
		t.Fatal(response.Body)
	}
	for i, event := range order.History {
		if event.ActorID != expected[i] {
			t.Fatal("history attribution changed", response.Body)
		}
	}
	grantReleaseRole(t, app, publisher, `["VIEWER"]`, "2", "history-publisher-revoked")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "history-execute"), 403, "permission_denied")
}

func TestPublicationFrozenChangesAndDescriptions(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	db := deliveryDB(t, driver)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "mutation_supplied_id_items", "", `{"content":{"id":"one","label":"initial"}}`))
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_supplied_id_items","items":[{"operation":"MODIFY","id":"one","expected_record_version":"1","content":{"label":"approved"}}]}`, "frozen")
	// The schema changed after approval. Failure retains the approved intent and target.
	deliveryExec(t, db, `ALTER TABLE mutation_supplied_id_items MODIFY label varchar(65) NOT NULL`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "frozen-execute"), 409, "release_frozen_changed")
	deliveryExec(t, db, `ALTER TABLE mutation_supplied_id_items MODIFY label varchar(64) NOT NULL`)
	mutationCode := "fixture_mutation_supplied_id_items_mutation_v1"
	renamed := policyIntegrationRequest(t, app, "PATCH", "/api/v1/mutation-policies/"+mutationCode+"/metadata", `{"name":"renamed","description":"description only"}`)
	if renamed.Code != 200 {
		t.Fatal(renamed.Body)
	}
	success := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "frozen-execute")
	publishedFixtureCommand(t, success)
	completePublicationFixture(t, app, path, "frozen-complete")
	path = approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_supplied_id_items","items":[{"operation":"MODIFY","id":"one","expected_record_version":"2","content":{"label":"stale"}}]}`, "stale-execution")
	deliveryExec(t, db, `UPDATE mutation_supplied_id_items SET label='newer' WHERE id='one'`)
	deliveryExec(t, db, `UPDATE rcc_record_versions SET lock_version=lock_version+1 WHERE table_name='mutation_supplied_id_items'`)
	stale := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "stale-execution")
	assertIntegrationErrorCode(t, stale, 409, "record_version_conflict")
	current := releaseRequest(t, app, "GET", path, "", "")
	if !strings.Contains(current.Body.String(), `"state":"APPROVED"`) {
		t.Fatal(current.Body)
	}
	row, version := recordVersionRow(t, app, "mutation_supplied_id_items", "one")
	if *row["label"] != "newer" || version != "3" {
		t.Fatal("stale execution wrote")
	}
	// Use a fresh, valid baseline so disable rejection cannot hide behind CAS.
	cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"3","reason":"replace stale proposal"}`, "disabled-cancel-stale")
	if cancelled.Code != 200 {
		t.Fatal(cancelled.Body)
	}
	path = approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_supplied_id_items","items":[{"operation":"MODIFY","id":"one","expected_record_version":"3","content":{"label":"should stay newer"}}]}`, "disabled-valid")
	off := policyIntegrationRequest(t, app, "POST", "/api/v1/table-policies/mutation_supplied_id_items/disable", "")
	if off.Code != 200 {
		t.Fatal(off.Body)
	}
	denied := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "inactive-execution")
	assertIntegrationErrorCode(t, denied, 403, "table_policy_disabled")
	current = releaseRequest(t, app, "GET", path, "", "")
	if !strings.Contains(current.Body.String(), `"state":"APPROVED"`) {
		t.Fatal(current.Body)
	}
	var targets int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id=?`, strings.TrimPrefix(path, "/api/v1/release-orders/")).Scan(&targets); err != nil || targets != 1 {
		t.Fatal("disable lost target", err)
	}
	var label string
	db.QueryRow(`SELECT label FROM mutation_supplied_id_items WHERE id='one'`).Scan(&label)
	if label != "newer" {
		t.Fatal("disable wrote business data")
	}

}

func TestPublicationActionCompetitionAndTableOrder(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	reviewer := publicationFixtureReviewer(t, app)
	session := integrationAdminSession(t, app)
	cookies, csrf := session.Result().Cookies(), sessionCSRF(t, session)
	for i, actions := range [][]string{{"execute", "cancel"}, {"execute", "execute"}} {
		path := approvePublication(t, app, reviewer, fmt.Sprintf(`{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"race-%d","label":"intent"}}]}`, i), fmt.Sprintf("race-%d", i))
		responses := make(chan *httptest.ResponseRecorder, 2)
		for j, action := range actions {
			go func(j int, action string) {
				body := `{"expected_version":"3"}`
				if action == "cancel" {
					body = `{"expected_version":"3","reason":"cancel concurrently"}`
				}
				responses <- accountRequestFrom(app, "POST", path+"/"+action, body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("race-%d-%d", i, j)})
			}(j, action)
		}
		successes := 0
		for range 2 {
			r := <-responses
			if r.Code == 200 {
				successes++
			} else if r.Code != 409 {
				t.Fatalf("race error %d %s", r.Code, r.Body)
			}
		}
		if successes != 1 {
			t.Fatal("more than one terminal action")
		}
		current := releaseRequest(t, app, "GET", path, "", "")
		var order domain.ReleaseOrder
		json.Unmarshal(current.Body.Bytes(), &order)
		query := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/query", fmt.Sprintf(`{"conditions":[{"field":"code","operator":"exact","value":"race-%d"}]}`, i))
		var result struct{ Rows []map[string]*string }
		json.Unmarshal(query.Body.Bytes(), &result)
		if (order.State == "SUCCEEDED") != (len(result.Rows) == 1) || order.Version != "4" {
			t.Fatal("terminal state disagrees with rows")
		}
	}
	paths := []string{}
	for i := range 2 {
		paths = append(paths, approvePublication(t, app, reviewer, fmt.Sprintf(`{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"ordered-%d","label":"intent"}}]}`, i), fmt.Sprintf("ordered-%d", i)))
	}
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i, path := range paths {
		go func(i int, path string) {
			responses <- accountRequestFrom(app, "POST", path+"/execute", `{"expected_version":"3"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("ordered-execute-%d", i)})
		}(i, path)
	}
	sequence := []int{}
	for range 2 {
		command := publishedFixtureCommand(t, <-responses)
		n, err := strconv.Atoi(command.Sequence)
		if err != nil || command.Sequence != command.TableVersion {
			t.Fatal("cursor/table disagreement")
		}
		sequence = append(sequence, n)
	}
	if sequence[0]-sequence[1] != 1 && sequence[1]-sequence[0] != 1 {
		t.Fatal("table commits did not allocate successive cursors", sequence)
	}
}

func TestPublicationSessionLocksHiddenForeignKeyDDL(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	rootDriver := *driver
	rootDriver.User = "root"
	owner := deliveryDB(t, &rootDriver)
	deliveryExec(t, owner, `CREATE DATABASE invisible_fk`)
	deliveryExec(t, owner, `CREATE TABLE invisible_fk.child(id int PRIMARY KEY,parent_id bigint unsigned) ENGINE=InnoDB`)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	ddl, err := owner.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ddl.Close()
	for i, checks := range []string{"1", "0"} {
		if _, err = ddl.ExecContext(ctx, "SET SESSION foreign_key_checks="+checks); err != nil {
			t.Fatal(err)
		}
		if _, err = ddl.ExecContext(ctx, "SET SESSION lock_wait_timeout=1"); err != nil {
			t.Fatal(err)
		}
		statement := fmt.Sprintf("ALTER TABLE invisible_fk.child ADD CONSTRAINT hidden_%d FOREIGN KEY(parent_id) REFERENCES rcc_test.mutation_delete_parents(id) ON DELETE CASCADE", i)
		err = app.mysql.ExecutePublication(ctx, func(session application.PublicationSession) error {
			if _, err := session.LockAndReadTableExecutionSchema(ctx, "mutation_delete_parents"); err != nil {
				return err
			}
			done := make(chan error, 1)
			go func() { _, err := ddl.ExecContext(ctx, statement); done <- err }()
			select {
			case err := <-done:
				var mysqlError *mysqldriver.MySQLError
				if !errors.As(err, &mysqlError) || mysqlError.Number != 1205 {
					t.Fatalf("DDL escaped publication session metadata lock: %v", err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("DDL did not honor bounded timeout")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = ddl.ExecContext(ctx, statement); err != nil {
			t.Fatal("same DDL after publication session commit", err)
		}
		if _, err = ddl.ExecContext(ctx, fmt.Sprintf("ALTER TABLE invisible_fk.child DROP FOREIGN KEY hidden_%d", i)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPublicationRejectsUnknownNonAutoIncrementIdentity(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `CREATE TABLE default_identity(id int NOT NULL DEFAULT 1 PRIMARY KEY,label varchar(30) NOT NULL) ENGINE=InnoDB`)
	deliveryExec(t, db, `INSERT INTO default_identity VALUES(0,'unrelated')`)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "default_identity", mutationPolicyFixture{AllowAdd: true})
	unknown := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"集成测试发布单","table_name":"default_identity","items":[{"operation":"ADD","content":{"label":"default identity"}}]}`, "unknown-default-id")
	assertIntegrationErrorCode(t, unknown, 422, "publication_unsupported")
	command := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "default_identity", "", `{"content":{"id":"1","label":"explicit identity"}}`))
	if command.ID != "1" {
		t.Fatal("explicit ID lost")
	}
	var unchanged string
	if err := db.QueryRow(`SELECT label FROM default_identity WHERE id=0`).Scan(&unchanged); err != nil || unchanged != "unrelated" {
		t.Fatal("unrelated identity touched", err)
	}
}

func TestPublicationStoredRowsAreVerifiedBeforeReadOrReplay(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"verify-storage","label":"trusted"}}]}`, "verify-storage")
	publishedFixtureCommand(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "verify-storage-execute"))
	id := strings.TrimPrefix(path, "/api/v1/release-orders/")
	var original []byte
	if err := db.QueryRow(`SELECT publication FROM rcc_release_details WHERE order_id=? AND position=0`, id).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for _, jsonPath := range []string{"$.final.checksum", "$.before.schema_digest", "$.id"} {
		deliveryExec(t, db, `UPDATE rcc_release_details SET publication=JSON_SET(publication,?,'damaged') WHERE order_id=? AND position=0`, jsonPath, id)
		for _, read := range []string{path} {
			assertIntegrationErrorCode(t, releaseRequest(t, app, "GET", read, "", ""), 503, "release_unavailable")
		}
		deliveryExec(t, db, `UPDATE rcc_release_details SET publication=? WHERE order_id=? AND position=0`, original, id)
	}
	deliveryExec(t, db, `UPDATE rcc_release_details SET publication=JSON_SET(publication,'$.final.fields[0].value','incorrect') WHERE order_id=?`, id)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "verify-storage-execute"), 503, "release_unavailable")
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM mutation_add_items WHERE code='verify-storage'`).Scan(&n)
	if n != 1 {
		t.Fatal("corrupt replay repeated business write")
	}
}
