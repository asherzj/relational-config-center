//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Initialize ordered pre-submit EDIT history below the original forward-result
// limit. Only the subsequent real rollback/cancellation requests consume headroom.
func seedLongPublishedHistory(t *testing.T, db *sql.DB, order domain.ReleaseOrder) domain.ReleaseOrder {
	t.Helper()
	tail := append([]domain.ReleaseEvent(nil), order.History[1:]...)
	order.History = order.History[:1]
	base, err := json.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, order.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	used, next := len(base), 2
	limit := application.ReleaseResultBytes - application.ReleaseContinuationHeadroom
	for used < limit-1400 {
		event := domain.ReleaseEvent{Action: "EDIT", ActorID: order.ApplicantID, At: at.Add(time.Duration(next) * time.Microsecond).Format(time.RFC3339Nano), Version: strconv.Itoa(next)}
		encoded, _ := json.Marshal(event)
		used += len(encoded) + 1
		order.History = append(order.History, event)
		next++
	}
	for _, event := range tail {
		event.Version = strconv.Itoa(next)
		event.At = at.Add(time.Duration(next) * time.Microsecond).Format(time.RFC3339Nano)
		order.History = append(order.History, event)
		next++
	}
	last := order.History[len(order.History)-1]
	order.Version, order.UpdatedAt = last.Version, last.At
	encoded, _ := json.Marshal(order)
	if len(encoded) > limit || order.State != "SUCCEEDED" || order.RollbackOrderID != "" {
		t.Fatal("unreachable forward capacity fixture")
	}
	if _, err := db.Exec(`UPDATE rcc_release_orders SET version=?,document=? WHERE id=?`, order.Version, encoded, order.ID); err != nil {
		t.Fatal(err)
	}
	return order
}

func nearLimitRollback(t *testing.T, app *adminApplication, original domain.ReleaseOrder, prefix string) domain.ReleaseOrder {
	t.Helper()
	path := "/api/v1/release-orders/" + original.ID
	for cycle := 0; cycle < 20; cycle++ {
		candidate := original
		candidate.RollbackPending = true
		candidate.RollbackOrderID = strings.Repeat("0", 32)
		version, _ := strconv.Atoi(candidate.Version)
		candidate.Version = strconv.Itoa(version + 1)
		candidate.UpdatedAt = "2026-09-08T06:00:00.123456Z"
		candidate.History = append(append([]domain.ReleaseEvent(nil), original.History...), domain.ReleaseEvent{Action: "ROLLBACK_REQUEST", ActorID: original.ApplicantID, At: candidate.UpdatedAt, Version: candidate.Version, RelatedOrderID: candidate.RollbackOrderID})
		encoded, _ := json.Marshal(candidate)
		room := application.ReleaseResultBytes - application.ReleaseTransportHeadroom - 4096 - 32 - len(encoded)
		if room < 6 {
			t.Fatalf("no valid next request in capacity fixture: %d", room)
		}
		final := room <= 12000
		length := 1000
		if final {
			length = room / 6
		}
		body, _ := json.Marshal(map[string]string{"expected_version": original.Version, "reason": strings.Repeat("<", length)})
		reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", string(body), fmt.Sprintf("%s-request-%d", prefix, cycle)), 201)
		if final {
			stored, err := app.mysql.GetReleaseOrder(t.Context(), original.ID)
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := json.Marshal(stored)
			if headroom := application.ReleaseResultBytes - application.ReleaseTransportHeadroom - 4096 - len(actual); headroom < 0 || headroom > 64 {
				t.Fatalf("pending boundary not reached: %d bytes", headroom)
			}
			t.Logf("accepted pending original: %d bytes after %d real request/cancel cycles", len(actual), cycle)
			return reverse
		}
		rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+reverse.ID+"/cancel", `{"expected_version":"1","reason":"next capacity cycle"}`, fmt.Sprintf("%s-cancel-%d", prefix, cycle)), 200)
		var err error
		original, err = app.mysql.GetReleaseOrder(t.Context(), original.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("capacity fixture did not converge")
	return domain.ReleaseOrder{}
}

func approveRollback(t *testing.T, app *adminApplication, reviewer *httptest.ResponseRecorder, order domain.ReleaseOrder, key string) string {
	t.Helper()
	path := "/api/v1/release-orders/" + order.ID
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, key+"-submit"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", `{"expected_version":"2","reason":"independent rollback review"}`, key+"-approve"), 200)
	return path
}

func rollbackOrderResponse(t *testing.T, response *httptest.ResponseRecorder, status int) domain.ReleaseOrder {
	t.Helper()
	if response.Code != status {
		t.Fatalf("release status %d, expected %d: %s", response.Code, status, response.Body)
	}
	var order domain.ReleaseOrder
	if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	return order
}

// The original can release a unique value and let a later item occupy it. Undo
// must unwind those dependencies before restoring the earlier business values.
func TestReleaseRollbackUnwindsUniqueValueDependencies(t *testing.T) {
	for _, operation := range []string{"DELETE", "MODIFY"} {
		t.Run(operation, func(t *testing.T) {
			app, db := batchEdgeApplication(t, `CREATE TABLE rollback_unique(id bigint PRIMARY KEY,code varchar(32) NOT NULL UNIQUE) ENGINE=InnoDB`, `INSERT INTO rollback_unique VALUES(1,'shared')`)
			enableMutationPolicy(t, app, "rollback_unique", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
			reviewer := publicationFixtureReviewer(t, app)
			content := `{}`
			if operation == "MODIFY" {
				content = `{"code":"released"}`
			}
			body := fmt.Sprintf(`{"title":"集成测试发布单","table_name":"rollback_unique","items":[{"operation":%q,"id":"1","expected_record_version":"0","content":%s},{"operation":"ADD","content":{"id":"2","code":"shared"}}]}`, operation, content)
			path := approvePublication(t, app, reviewer, body, "unique-dependency")
			rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "unique-forward"), 200)
			reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore unique value owner"}`, "unique-apply"), 201)
			reversePath := approveRollback(t, app, reviewer, reverse, "unique-reverse")
			result := rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "unique-execute"), 200)
			if result.Publication.Commands[0].ID != "2" || result.Publication.Commands[1].ID != "1" {
				t.Fatal("reverse publication did not unwind original execution order")
			}
			row, version := recordVersionRow(t, app, "rollback_unique", "1")
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM rollback_unique`).Scan(&count); err != nil || count != 1 || row["code"] == nil || *row["code"] != "shared" || version != "2" {
				t.Fatalf("unique owner not restored: %v %s count=%d err=%v", row, version, count, err)
			}
			if source := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200); source.State != "ROLLED_BACK" || source.RollbackOrderID != reverse.ID {
				t.Fatal("unique inverse did not finish original")
			}
		})
	}
}

// AC-041: the public authenticated workflow reverses every actual mixed result,
// keeps original history, and returns the original execute result on old-key retry.
func TestReleaseRollbackMixedPublication(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	db := deliveryDB(t, driver)
	if _, err := db.Exec(`INSERT INTO mutation_add_items(id,code,label,metadata) VALUES(10,'modify','old','null'),(20,'delete','saved','{"x":1}')`); err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"修正渠道配置","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"new","label":"added"}},{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"modified"}},{"operation":"DELETE","id":"20","expected_record_version":"0","content":{}}]}`, "rollback-mixed")
	forward := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-forward")
	original := rollbackOrderResponse(t, forward, 200)
	reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore previous configuration"}`, "rollback-apply"), 201)
	if reverse.Title != "回滚：修正渠道配置" || reverse.State != "DRAFT" || len(reverse.Items) != 3 || reverse.Items[0].Operation != "ADD" || reverse.Items[1].Operation != "MODIFY" || reverse.Items[2].Operation != "DELETE" {
		t.Fatalf("reverse: %+v", reverse)
	}
	for index, item := range reverse.Items {
		if item.ID == nil || string(*item.ID) != original.Publication.Commands[len(original.Publication.Commands)-1-index].ID || item.ExpectedRecordVersion != "1" {
			t.Fatalf("baseline %d: %+v", index, item)
		}
	}
	reversePath := "/api/v1/release-orders/" + reverse.ID
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"1"}`, "rollback-unapproved"), 422, "release_state_invalid")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/submit", `{"expected_version":"1"}`, "rollback-submit"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", reversePath+"/approve", `{"expected_version":"2","reason":"independent rollback review"}`, "rollback-approve"), 200)
	rolled := rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "rollback-execute"), 200)
	if rolled.State != "SUCCEEDED" || rolled.Publication.TableVersion != "2" {
		t.Fatalf("result: %+v", rolled)
	}
	for _, command := range rolled.Publication.Commands {
		if command.RecordVersion != "2" {
			t.Fatalf("reverse version %s", command.RecordVersion)
		}
	}
	for id, label := range map[string]string{"10": "old", "20": "saved"} {
		row, version := recordVersionRow(t, app, "mutation_add_items", id)
		if version != "2" || *row["label"] != label {
			t.Fatalf("restored %s %v %s", id, row, version)
		}
	}
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "ROLLED_BACK" || current.Publication.Commands[0].ID != original.Publication.Commands[0].ID {
		t.Fatal("original history lost", current)
	}
	replay := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-forward"), 200)
	if replay.State != "SUCCEEDED" || replay.Version != "4" || replay.Publication.Commands[0].ID != original.Publication.Commands[0].ID {
		t.Fatal("old execute result changed", replay)
	}
	var rows, targets, notifications, commands int
	for query, dest := range map[string]*int{`SELECT COUNT(*) FROM mutation_add_items`: &rows, `SELECT COUNT(*) FROM rcc_release_targets`: &targets, `SELECT COUNT(*) FROM rcc_refresh_notifications`: &notifications, `SELECT COUNT(*) FROM rcc_publication_commands`: &commands} {
		if err := db.QueryRow(query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if rows != 2 || targets != 0 || notifications != 2 || commands != 6 {
		t.Fatalf("atomic effects rows=%d targets=%d notifications=%d commands=%d", rows, targets, notifications, commands)
	}
}

// AC-042/043: a supported target-row trigger must not turn a nominal reverse
// UPDATE into a falsely successful restoration. Every transaction effect rolls back.
func TestReleaseRollbackRejectsChangedRestoreValue(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	for _, sql := range []string{`INSERT INTO mutation_add_items(id,code,label,quantity) VALUES(10,'trigger','original',10)`, `CREATE TRIGGER restore_quantity BEFORE UPDATE ON mutation_add_items FOR EACH ROW SET NEW.quantity=NEW.quantity+1`} {
		if _, err := db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	original := rollbackOrderResponse(t, publicationFixtureRequest(t, app, "MODIFY", "mutation_add_items", "10", `{"expected_version":"0","content":{"label":"published"}}`), 200)
	path := "/api/v1/release-orders/" + original.ID
	reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore business values"}`, "trigger-rollback"), 201)
	reversePath := "/api/v1/release-orders/" + reverse.ID
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/submit", `{"expected_version":"1"}`, "trigger-submit"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", reversePath+"/approve", `{"expected_version":"2","reason":"checked"}`, "trigger-approve"), 200)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "trigger-execute"), 422, "rollback_restore_mismatch")
	row, version := recordVersionRow(t, app, "mutation_add_items", "10")
	if version != "1" || *row["label"] != "published" || *row["quantity"] != "11" {
		t.Fatalf("false restoration escaped: %v %s", row, version)
	}
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "SUCCEEDED" || !current.RollbackPending {
		t.Fatal("original falsely closed", current)
	}
	approved := rollbackOrderResponse(t, releaseRequest(t, app, "GET", reversePath, "", ""), 200)
	if approved.State != "APPROVED" || approved.Version != "3" {
		t.Fatal("approval lost", approved)
	}
	var targets, notifications, requests, commands int
	for sql, dest := range map[string]*int{`SELECT COUNT(*) FROM rcc_release_targets`: &targets, `SELECT COUNT(*) FROM rcc_refresh_notifications`: &notifications, `SELECT COUNT(*) FROM rcc_publication_commands`: &commands, `SELECT COUNT(*) FROM rcc_release_requests WHERE request_key='trigger-execute'`: &requests} {
		if err := db.QueryRow(sql).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if targets != 1 || notifications != 1 || commands != 1 || requests != 0 {
		t.Fatalf("partial rollback effects %d %d %d %d", targets, notifications, commands, requests)
	}
}

// AC-044: original-row serialization converges independent requests and old-key
// retries, while cancellation/rejection releases the same association atomically.
func TestReleaseRollbackConcurrencyAndReapplication(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	original := rollbackOrderResponse(t, publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"code":"concurrent","label":"saved"}}`), 200)
	path := "/api/v1/release-orders/" + original.ID
	session := integrationAdminSession(t, app)
	csrf, cookies := sessionCSRF(t, session), session.Result().Cookies()
	body := `{"expected_version":"4","reason":"concurrent rollback"}`
	responses := make(chan *httptest.ResponseRecorder, 3)
	for _, key := range []string{"rollback-same-key", "rollback-same-key", "rollback-other-key"} {
		go func(key string) {
			responses <- accountRequestFrom(app, "POST", path+"/rollback", body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
		}(key)
	}
	var reverse domain.ReleaseOrder
	winners, conflicts := 0, 0
	for range 3 {
		response := <-responses
		if response.Code == 201 {
			order := rollbackOrderResponse(t, response, 201)
			if reverse.ID != "" && reverse.ID != order.ID {
				t.Fatal("duplicate valid rollback", reverse.ID, order.ID)
			}
			reverse = order
			winners++
		} else {
			assertIntegrationErrorCode(t, response, 409, "release_version_conflict")
			conflicts++
		}
	}
	if winners == 0 || conflicts == 0 {
		t.Fatalf("winners %d conflicts %d", winners, conflicts)
	}
	reversePath := "/api/v1/release-orders/" + reverse.ID
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.RollbackOrderID != reverse.ID || !current.RollbackPending || current.Version != "5" || reverse.RollbackOfID != original.ID {
		t.Fatal("missing association", current, reverse)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"5","reason":"another request"}`, "rollback-conflict"), 409, "rollback_conflict")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", reversePath, `{"title":"集成测试发布单","table_name":"mutation_add_items","expected_version":"1","items":[{"operation":"DELETE","id":"1","expected_record_version":"1","content":{}}]}`, "rollback-edit"), 422, "rollback_locked")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/cancel", `{"expected_version":"1","reason":"cancel before publishing"}`, "rollback-cancel"), 200)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", reversePath+"/copy", `{"expected_version":"2","confirmed":true,"items":[]}`, "rollback-copy"), 422, "rollback_locked")
	current = rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "SUCCEEDED" || current.RollbackPending || current.RollbackOrderID != reverse.ID || current.History[len(current.History)-1].RelatedOrderID != reverse.ID {
		t.Fatal("cancel linkage lost", current)
	}
	request := fmt.Sprintf(`{"expected_version":%q,"reason":"reapply after cancellation"}`, current.Version)
	nextResponse := releaseRequest(t, app, "POST", path+"/rollback", request, "rollback-reapply")
	next := rollbackOrderResponse(t, nextResponse, 201)
	replay := releaseRequest(t, app, "POST", path+"/rollback", request, "rollback-reapply")
	if replay.Code != 201 || replay.Body.String() != nextResponse.Body.String() || next.ID == reverse.ID {
		t.Fatal("reapplication retry changed", replay.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/rollback", strings.Replace(request, "reapply after cancellation", "changed reason", 1), "rollback-reapply"), 409, "idempotency_conflict")
	nextPath := "/api/v1/release-orders/" + next.ID
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", nextPath+"/submit", `{"expected_version":"1"}`, "rollback-reject-submit"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, publicationFixtureReviewer(t, app), "POST", nextPath+"/reject", `{"expected_version":"2","reason":"needs another independent review"}`, "rollback-reject"), 200)
	current = rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.RollbackPending {
		t.Fatal("rejection left active association")
	}
	last := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", fmt.Sprintf(`{"expected_version":%q,"reason":"final request"}`, current.Version), "rollback-final-apply"), 201)
	lastPath := approveRollback(t, app, publicationFixtureReviewer(t, app), last, "rollback-final")
	for _, key := range []string{"rollback-final-execute", "rollback-final-execute", "rollback-execute-compete"} {
		go func(key string) {
			responses <- accountRequestFrom(app, "POST", lastPath+"/execute", `{"expected_version":"3"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
		}(key)
	}
	winners, conflicts = 0, 0
	for range 3 {
		response := <-responses
		if response.Code == 200 {
			winners++
			result := rollbackOrderResponse(t, response, 200)
			if result.Publication.TableVersion != "2" {
				t.Fatal("duplicate effective rollback")
			}
		} else {
			assertIntegrationErrorCode(t, response, 409, "release_version_conflict")
			conflicts++
		}
	}
	if winners == 0 || conflicts == 0 {
		t.Fatalf("execution winners %d conflicts %d", winners, conflicts)
	}
	current = rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "ROLLED_BACK" || current.RollbackPending || current.RollbackOrderID != last.ID {
		t.Fatal("success linkage lost", current)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/rollback", fmt.Sprintf(`{"expected_version":%q,"reason":"duplicate"}`, current.Version), "rollback-after-success"), 422, "release_state_invalid")
	old := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", request, "rollback-reapply"), 201)
	if old.ID != next.ID || old.State != "DRAFT" {
		t.Fatal("old application result rewritten", old)
	}
}

// AC-041: a deleted ENUM primary key is recovered from its saved comparison
// identity, retaining the tombstone version instead of inventing a new key.
func TestReleaseRollbackRestoresDeletedEnumIdentity(t *testing.T) {
	app, db := batchEdgeApplication(t, `CREATE TABLE rollback_enum(id enum('draft','active','paused') PRIMARY KEY,label varchar(40) NOT NULL) ENGINE=InnoDB`, `INSERT INTO rollback_enum VALUES('active','retained'),('paused','waiting')`)
	enableMutationPolicy(t, app, "rollback_enum", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"rollback_enum","items":[{"operation":"DELETE","id":"active","expected_record_version":"0","content":{}},{"operation":"DELETE","id":"paused","expected_record_version":"0","content":{}}]}`, "enum-forward")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "enum-forward-execute"), 200)
	reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore deleted enum identity"}`, "enum-rollback"), 201)
	if reverse.Items[0].ID == nil || *reverse.Items[0].ID != "paused" || reverse.Items[1].ID == nil || *reverse.Items[1].ID != "active" || reverse.Items[0].ExpectedRecordVersion != "1" || reverse.Items[1].ExpectedRecordVersion != "1" {
		t.Fatal("tombstone changed", reverse)
	}
	reversePath := approveRollback(t, app, publicationFixtureReviewer(t, app), reverse, "enum-rollback")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "enum-execute"), 200)
	for id, label := range map[string]string{"active": "retained", "paused": "waiting"} {
		row, version := recordVersionRow(t, app, "rollback_enum", id)
		if version != "2" || *row["label"] != label {
			t.Fatal("identity lost", row, version)
		}
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_record_versions WHERE table_name='rollback_enum' AND lock_version=2`: 2})
}

// AC-043: server-held before values are not new uploaded fields. A small reverse
// request restores a >64 KiB business value, with current audit and generated data.
func TestReleaseRollbackRestoresBusinessFieldsWithNewAudit(t *testing.T) {
	app, db := batchEdgeApplication(t, `CREATE TABLE rollback_audit(id bigint PRIMARY KEY,label varchar(40) NOT NULL,payload LONGTEXT NOT NULL,creator varchar(64) NOT NULL,created_at datetime(6) NOT NULL,modifier varchar(64) NOT NULL,modified_at datetime(6) NOT NULL,moment timestamp(6) NOT NULL,derived varchar(100) GENERATED ALWAYS AS(CONCAT(label,':old')) STORED) ENGINE=InnoDB`, `INSERT INTO rollback_audit(id,label,payload,creator,created_at,modifier,modified_at,moment) VALUES(1,'original',REPEAT('x',200000),'old-creator','2020-01-02 03:04:05.123456','old-modifier','2020-02-03 04:05:06.123456','2020-03-04 05:06:07.123456'),(2,'deleted',REPEAT('y',200000),'old-creator','2020-01-02 03:04:05.123456','old-modifier','2020-02-03 04:05:06.123456','2020-03-04 05:06:07.123456')`)
	creator, created, modifier, modified := "creator", "created_at", "modifier", "modified_at"
	enableMutationPolicy(t, app, "rollback_audit", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true, CreateOperatorField: &creator, CreateTimeField: &created, ModifyOperatorField: &modifier, ModifyTimeField: &modified})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"rollback_audit","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"label":"published","payload":"short"}},{"operation":"DELETE","id":"2","expected_record_version":"0","content":{}}]}`, "audit-forward")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "audit-forward-execute"), 200)
	deliveryExec(t, db, `ALTER TABLE rollback_audit MODIFY derived varchar(100) GENERATED ALWAYS AS(CONCAT(label,':new')) STORED`)
	reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore with current audit"}`, "audit-rollback"), 201)
	if len(*reverse.Items[0].Content["payload"]) != 200000 {
		t.Fatal("historical field was truncated")
	}
	for _, item := range reverse.Items {
		for _, name := range []string{"creator", "created_at", "modifier", "modified_at", "derived"} {
			if _, exists := item.Content[name]; exists {
				t.Fatal("restoration blindly copied automatic/generated value", name)
			}
		}
	}
	reversePath := approveRollback(t, app, reviewer, reverse, "audit-rollback")
	publisher := registerAccount(t, app, "rollback.publisher", "rollback.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "rollback-publisher-role")
	result := rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "audit-rollback-execute"), 200)
	for id, label := range map[string]string{"1": "original", "2": "deleted"} {
		row, version := recordVersionRow(t, app, "rollback_audit", id)
		if version != "2" || *row["label"] != label || len(*row["payload"]) != 200000 || *row["derived"] != label+":new" || *row["modifier"] != accountID(t, publisher) || *row["moment"] != "2020-03-04T05:06:07.123456Z" {
			t.Fatalf("incorrect restored business row %s %v %s", id, row, version)
		}
		if id == "2" && (*row["creator"] != accountID(t, publisher) || *row["created_at"] == "2020-01-02 03:04:05.123456") {
			t.Fatal("old creation audit copied")
		}
		if *row["modified_at"] == "2020-02-03 04:05:06.123456" {
			t.Fatal("old modify time copied")
		}
	}
	if result.Publication.PublisherID != accountID(t, publisher) || result.Publication.ExecutedAt == original.Publication.ExecutedAt {
		t.Fatal("rollback publisher/time lost")
	}
	// A formerly generated value does not become user-supplied historical input
	// merely because a later schema makes that column writable with a new default.
	deleted := rollbackOrderResponse(t, publicationFixtureRequest(t, app, "DELETE", "rollback_audit", "2", `{"expected_version":"2","content":{}}`), 200)
	deliveryExec(t, db, `ALTER TABLE rollback_audit MODIFY derived varchar(100) NOT NULL DEFAULT 'current-default'`)
	last := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+deleted.ID+"/rollback", `{"expected_version":"4","reason":"respect formerly generated field"}`, "former-generated-apply"), 201)
	if _, copied := last.Items[0].Content["derived"]; copied {
		t.Fatal("former generated value copied as ordinary input")
	}
	lastPath := approveRollback(t, app, reviewer, last, "former-generated")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", lastPath+"/execute", `{"expected_version":"3"}`, "former-generated-execute"), 200)
	row, version := recordVersionRow(t, app, "rollback_audit", "2")
	if *row["derived"] != "current-default" || version != "4" {
		t.Fatal("former generated field did not use current semantics")
	}
}

// AC-044/capacity: a valid pending association at its own continuation limit
// must still accept cancellation/rejection and release the original for review.
func TestReleaseRollbackPendingHistoryCanTerminate(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	for _, action := range []string{"cancel", "reject"} {
		t.Run(action, func(t *testing.T) {
			original := rollbackOrderResponse(t, publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", fmt.Sprintf(`{"content":{"code":%q,"label":"capacity"}}`, action)), 200)
			path := "/api/v1/release-orders/" + original.ID
			stored, err := app.mysql.GetReleaseOrder(t.Context(), original.ID)
			if err != nil {
				t.Fatal(err)
			}
			stored = seedLongPublishedHistory(t, db, stored)
			reverse := nearLimitRollback(t, app, stored, action+"-capacity")
			reversePath := "/api/v1/release-orders/" + reverse.ID
			version := "1"
			if action == "reject" {
				rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/submit", `{"expected_version":"1"}`, "capacity-submit"), 200)
				version = "2"
			}
			body, _ := json.Marshal(map[string]string{"expected_version": version, "reason": strings.Repeat("<", 2000)})
			var response *httptest.ResponseRecorder
			if action == "reject" {
				response = releaseActorRequest(t, app, publicationFixtureReviewer(t, app), "POST", reversePath+"/reject", string(body), "capacity-reject")
			} else {
				response = releaseRequest(t, app, "POST", reversePath+"/cancel", string(body), "capacity-cancel")
			}
			rollbackOrderResponse(t, response, 200)
			current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
			if current.State != "SUCCEEDED" || current.RollbackPending {
				t.Fatal("accepted pending association cannot terminate")
			}
			batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 0})
		})
	}
}

// AC-042: neither current-data review nor another action can refresh the original
// post-publication token. A stale later member leaves the complete batch intact.
func TestReleaseRollbackRejectsLaterChanges(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	for index, stage := range []string{"create", "submit", "execute", "delete-recreate", "maintenance"} {
		t.Run(stage, func(t *testing.T) {
			id := fmt.Sprint(100 + index*2)
			other := fmt.Sprint(101 + index*2)
			deliveryExec(t, db, fmt.Sprintf(`INSERT INTO mutation_add_items(id,code,label) VALUES(%s,'old-%s','old'),(%s,'old-%s','old')`, id, id, other, other))
			path := approvePublication(t, app, reviewer, fmt.Sprintf(`{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":%q,"expected_record_version":"0","content":{"label":"published"}},{"operation":"MODIFY","id":%q,"expected_record_version":"0","content":{"label":"published"}}]}`, id, other), stage+"-forward")
			original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, stage+"-forward-execute"), 200)
			var reverse domain.ReleaseOrder
			if stage == "submit" || stage == "execute" {
				reverse = rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore original"}`, stage+"-apply"), 201)
			}
			reversePath := "/api/v1/release-orders/" + reverse.ID
			if stage == "execute" {
				approveRollback(t, app, reviewer, reverse, stage+"-reverse")
			}
			switch stage {
			case "delete-recreate":
				rollbackOrderResponse(t, publicationFixtureRequest(t, app, "DELETE", "mutation_add_items", id, `{"expected_version":"1","content":{}}`), 200)
				rollbackOrderResponse(t, publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", fmt.Sprintf(`{"content":{"id":%q,"code":"rebuilt-%s","label":"newer"}}`, id, id)), 200)
			case "execute":
				// A controlled maintenance writer advances the same persisted token.
				stored, err := app.mysql.GetReleaseOrder(t.Context(), original.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE mutation_add_items SET label='newer' WHERE id=?`, id); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE rcc_record_versions SET lock_version=lock_version+1 WHERE table_name=? AND record_key=?`, stored.Items[0].RecordTable, stored.Items[0].RecordKey); err != nil {
					t.Fatal(err)
				}
			case "maintenance":
				deliveryExec(t, db, `INSERT INTO rcc_record_versions(table_name,record_key,lock_version) VALUES('mutation_add_items',X'',1000)`)
			default:
				rollbackOrderResponse(t, publicationFixtureRequest(t, app, "MODIFY", "mutation_add_items", id, `{"expected_version":"1","content":{"label":"newer"}}`), 200)
			}
			var failed *httptest.ResponseRecorder
			if stage == "submit" {
				failed = releaseRequest(t, app, "POST", reversePath+"/submit", `{"expected_version":"1"}`, stage+"-stale")
			} else if stage == "execute" {
				failed = releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, stage+"-stale")
			} else {
				failed = releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore original"}`, stage+"-stale")
			}
			assertIntegrationErrorCode(t, failed, 409, "record_version_conflict")
			if stage != "maintenance" {
				batchEdgeIndex(t, failed, 1)
			}
			row, version := recordVersionRow(t, app, "mutation_add_items", other)
			if *row["label"] != "published" || stage != "maintenance" && version != "1" {
				t.Fatal("earlier member changed", row, version)
			}
			current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
			if current.State != "SUCCEEDED" {
				t.Fatal("stale reverse marked original")
			}
			if stage == "submit" {
				preview := releaseRequest(t, app, "POST", "/api/v1/release-orders/preview", fmt.Sprintf(`{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":%q,"content":{"label":"old"}}]}`, id), "")
				if preview.Code != 200 || !strings.Contains(preview.Body.String(), `"expected_record_version":"2"`) {
					t.Fatal("current review unavailable", preview.Body)
				}
				assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", reversePath, fmt.Sprintf(`{"title":"集成测试发布单","table_name":"mutation_add_items","expected_version":"1","items":[{"operation":"MODIFY","id":%q,"expected_record_version":"2","content":{"label":"old"}}]}`, id), "reverse-cannot-refresh"), 422, "rollback_locked")
				assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", reversePath+"/submit", `{"expected_version":"1"}`, "reverse-still-stale"), 409, "record_version_conflict")
			}
		})
	}
}

func TestReleaseRollbackRejectsNewUniqueConstraintAtomically(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(100,'first','same'),(200,'second','same')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"100","expected_record_version":"0","content":{"label":"one"}},{"operation":"MODIFY","id":"200","expected_record_version":"0","content":{"label":"two"}}]}`, "constraint-forward")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "constraint-forward-execute"), 200)
	deliveryExec(t, db, `ALTER TABLE mutation_add_items ADD UNIQUE KEY new_label_unique(label)`)
	reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"4","reason":"restore before new constraint"}`, "constraint-reverse"), 201)
	reversePath := approveRollback(t, app, reviewer, reverse, "constraint-reverse")
	failed := releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "constraint-execute")
	assertIntegrationErrorCode(t, failed, 409, "duplicate_key")
	batchEdgeIndex(t, failed, 1)
	for id, label := range map[string]string{"100": "one", "200": "two"} {
		row, v := recordVersionRow(t, app, "mutation_add_items", id)
		if *row["label"] != label || v != "1" {
			t.Fatal("partial restoration", row, v)
		}
	}
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "SUCCEEDED" || !current.RollbackPending {
		t.Fatal("constraint failure closed original")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 2, `SELECT COUNT(*) FROM rcc_publication_commands`: 2, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 1, `SELECT COUNT(*) FROM rcc_release_requests WHERE request_key='constraint-execute'`: 0, `SELECT table_version FROM rcc_table_publications WHERE table_name='mutation_add_items'`: 1})
}

func TestReleaseRollbackUsesCurrentIndependentRoles(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	original := rollbackOrderResponse(t, publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"code":"role-test","label":"published"}}`), 200)
	path := "/api/v1/release-orders/" + original.ID
	editor := registerAccount(t, app, "rollback.editor", "rollback.editor@example.com", "correct horse battery staple")
	for index, roles := range []string{`["VIEWER"]`, `["APPROVER"]`, `["PUBLISHER"]`} {
		if index > 0 {
			grantReleaseRole(t, app, editor, roles, fmt.Sprint(index), fmt.Sprintf("role-rollback-%d", index))
		}
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", path+"/rollback", `{"expected_version":"4","reason":"requires EDITOR"}`, "role-rollback-apply"), 403, "permission_denied")
	}
	grantReleaseRole(t, app, editor, `["EDITOR","APPROVER"]`, "3", "role-rollback-editor")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", path+"/rollback", `{"expected_version":"4","reason":"   "}`, "role-empty-reason"), 422, "release_invalid")
	applyBody := `{"expected_version":"4","reason":"another editor may request"}`
	reverse := rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/rollback", applyBody, "role-rollback-apply"), 201)
	if reverse.ApplicantID != accountID(t, editor) || reverse.ApplicantID == original.ApplicantID {
		t.Fatal("wrong reverse applicant")
	}
	reversePath := "/api/v1/release-orders/" + reverse.ID
	rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", reversePath+"/submit", `{"expected_version":"1"}`, "role-reverse-submit"), 200)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", reversePath+"/approve", `{"expected_version":"2","reason":"self approval"}`, "role-reverse-self"), 403, "permission_denied")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/approve", `{"expected_version":"2","reason":"original applicant independently approves"}`, "role-reverse-approve"), 200)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "role-reverse-publish"), 403, "permission_denied")
	grantReleaseRole(t, app, editor, `["VIEWER"]`, "4", "role-reverse-revoke")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", path+"/rollback", applyBody, "role-rollback-apply"), 403, "permission_denied")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "role-reverse-execute"), 200)
	current := rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "GET", reversePath, "", ""), 200)
	if current.ApplicantID != accountID(t, editor) || current.History[0].ActorID != accountID(t, editor) {
		t.Fatal("revocation rewrote permanent history")
	}
}

// Real storage faults cover the association-specific transaction writes. Existing
// publication tests already cover Command, notification and commit uncertainty.
func TestReleaseRollbackAssociationFailuresAreAtomic(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	original := rollbackOrderResponse(t, publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"code":"fault","label":"retained"}}`), 200)
	path := "/api/v1/release-orders/" + original.ID
	deliveryExec(t, db, `CREATE TRIGGER fail_reverse_insert BEFORE INSERT ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test storage boundary'`)
	body := `{"expected_version":"4","reason":"fault recovery"}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/rollback", body, "fault-apply"), 503, "release_unavailable")
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.Version != "4" || current.RollbackPending || current.RollbackOrderID != "" {
		t.Fatal("failed creation left source linkage")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_orders`: 1, `SELECT COUNT(*) FROM rcc_release_requests WHERE request_key='fault-apply'`: 0})
	deliveryExec(t, db, `DROP TRIGGER fail_reverse_insert`)
	reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback", body, "fault-apply"), 201)
	reversePath := approveRollback(t, app, publicationFixtureReviewer(t, app), reverse, "fault-reverse")
	deliveryExec(t, db, `CREATE TRIGGER fail_rollback_marker BEFORE UPDATE ON rcc_release_orders FOR EACH ROW BEGIN IF NEW.state='ROLLED_BACK' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test storage boundary'; END IF; END`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "fault-execute"), 503, "release_unavailable")
	current = rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "SUCCEEDED" || current.Version != "5" || !current.RollbackPending {
		t.Fatal("failed marker changed original")
	}
	row, version := recordVersionRow(t, app, "mutation_add_items", original.Publication.Commands[0].ID)
	if version != "1" || *row["label"] != "retained" {
		t.Fatal("reverse data committed without original marker")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 1, `SELECT COUNT(*) FROM rcc_publication_commands`: 1, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 1, `SELECT COUNT(*) FROM rcc_release_requests WHERE request_key='fault-execute'`: 0, `SELECT table_version FROM rcc_table_publications WHERE table_name='mutation_add_items'`: 1})
	deliveryExec(t, db, `DROP TRIGGER fail_rollback_marker`)
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", reversePath+"/execute", `{"expected_version":"3"}`, "fault-execute"), 200)
	current = rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "ROLLED_BACK" {
		t.Fatal("same-key fault recovery did not complete")
	}
}

func TestReleaseRollbackRestoresStoredTimeDuration(t *testing.T) {
	app, _ := batchEdgeApplication(t, `CREATE TABLE rollback_duration(id bigint PRIMARY KEY,label varchar(40) NOT NULL,duration TIME(6) NOT NULL) ENGINE=InnoDB`, `INSERT INTO rollback_duration VALUES(1,'historical','-120:30:40.123456')`)
	enableMutationPolicy(t, app, "rollback_duration", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	original := rollbackOrderResponse(t, publicationFixtureRequest(t, app, "DELETE", "rollback_duration", "1", `{"expected_version":"0","content":{}}`), 200)
	reverse := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+original.ID+"/rollback", `{"expected_version":"4","reason":"restore complete stored duration"}`, "duration-rollback"), 201)
	path := approveRollback(t, app, publicationFixtureReviewer(t, app), reverse, "duration-rollback")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "duration-execute"), 200)
	row, version := recordVersionRow(t, app, "rollback_duration", "1")
	if *row["duration"] != "-120:30:40.123456" || version != "2" {
		t.Fatal("historical duration changed", row, version)
	}
}
