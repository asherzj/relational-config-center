//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Large accepted history must not prevent either terminal action.
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
	if len(encoded) > limit || (order.State != "SUCCEEDED" && order.State != "COMPLETED") || order.RollbackOrderID != "" {
		t.Fatal("unreachable forward capacity fixture")
	}
	seedReleaseWorkflowHistory(t, db, order)
	return order
}

// Capacity fixtures replace only workflow metadata, retaining real detail and
// execution storage created through the ordinary HTTP lifecycle.
func seedReleaseWorkflowHistory(t *testing.T, db *sql.DB, order domain.ReleaseOrder) {
	t.Helper()
	if _, err := db.Exec(`UPDATE rcc_release_orders SET version=?,document=JSON_SET(document,'$.history',CAST(? AS JSON),'$.version',?,'$.updated_at',?) WHERE id=?`, order.Version, mustJSONHistory(order.History), order.Version, order.UpdatedAt, order.ID); err != nil {
		t.Fatal(err)
	}
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

func quickRestoreFixture(t *testing.T, app *adminApplication, path, key string) domain.ReleaseOrder {
	t.Helper()
	actor := integrationAdminSession(t, app)
	original := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	preview := readQuickPreview(t, app, actor, path, original.Version)
	return rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody(original.Version, preview.Digest, ""), key), 200)
}

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
			result := quickRestoreFixture(t, app, path, "unique-restore")
			if result.Rollback.Commands[0].ID != "2" || result.Rollback.Commands[1].ID != "1" {
				t.Fatal("reverse publication did not unwind original execution order")
			}
			row, version := recordVersionRow(t, app, "rollback_unique", "1")
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM rollback_unique`).Scan(&count); err != nil || count != 1 || row["code"] == nil || *row["code"] != "shared" || version != "2" {
				t.Fatalf("unique owner not restored: %v %s count=%d err=%v", row, version, count, err)
			}
			if source := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200); source.State != "ROLLED_BACK" || source.ID != result.ID {
				t.Fatal("unique inverse did not finish original")
			}
		})
	}
}

// AC-041: the public authenticated workflow reverses every actual mixed result,
// keeps original history, and returns the original execute result on old-key retry.

func TestReleaseRollbackRestoresDeletedEnumIdentity(t *testing.T) {
	app, db := batchEdgeApplication(t, `CREATE TABLE rollback_enum(id enum('draft','active','paused') PRIMARY KEY,label varchar(40) NOT NULL) ENGINE=InnoDB`, `INSERT INTO rollback_enum VALUES('active','retained'),('paused','waiting')`)
	enableMutationPolicy(t, app, "rollback_enum", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"rollback_enum","items":[{"operation":"DELETE","id":"active","expected_record_version":"0","content":{}},{"operation":"DELETE","id":"paused","expected_record_version":"0","content":{}}]}`, "enum-forward")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "enum-forward-execute"), 200)
	result := quickRestoreFixture(t, app, path, "enum-restore")
	if result.Rollback.Commands[0].ID != "paused" || result.Rollback.Commands[1].ID != "active" {
		t.Fatal("deleted ENUM identities or inverse order changed")
	}

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
	app, _ := batchEdgeApplication(t, `CREATE TABLE rollback_audit(id bigint PRIMARY KEY,label varchar(40) NOT NULL,payload LONGTEXT NOT NULL,creator varchar(64) NOT NULL,created_at datetime(6) NOT NULL,modifier varchar(64) NOT NULL,modified_at datetime(6) NOT NULL,moment timestamp(6) NOT NULL,derived varchar(100) GENERATED ALWAYS AS(CONCAT(label,':old')) STORED) ENGINE=InnoDB`, `INSERT INTO rollback_audit(id,label,payload,creator,created_at,modifier,modified_at,moment) VALUES(1,'original',REPEAT('x',200000),'old-creator','2020-01-02 03:04:05.123456','old-modifier','2020-02-03 04:05:06.123456','2020-03-04 05:06:07.123456'),(2,'deleted',REPEAT('y',200000),'old-creator','2020-01-02 03:04:05.123456','old-modifier','2020-02-03 04:05:06.123456','2020-03-04 05:06:07.123456')`)
	creator, created, modifier, modified := "creator", "created_at", "modifier", "modified_at"
	enableMutationPolicy(t, app, "rollback_audit", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true, CreateOperatorField: &creator, CreateTimeField: &created, ModifyOperatorField: &modifier, ModifyTimeField: &modified})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"rollback_audit","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"label":"published","payload":"short"}},{"operation":"DELETE","id":"2","expected_record_version":"0","content":{}}]}`, "audit-forward")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "audit-forward-execute"), 200)
	actor := integrationAdminSession(t, app)
	reverse := readQuickPreview(t, app, actor, path, "4")
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
	publisher := registerAccount(t, app, "rollback.publisher", "rollback.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "rollback-publisher-role")
	result := rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback", quickRollbackBody("4", reverse.Digest, ""), "audit-restore"), 200)
	for id, label := range map[string]string{"1": "original", "2": "deleted"} {
		row, version := recordVersionRow(t, app, "rollback_audit", id)
		if version != "2" || *row["label"] != label || len(*row["payload"]) != 200000 || *row["derived"] != label+":old" || *row["modifier"] != accountID(t, publisher) || *row["moment"] != "2020-03-04T05:06:07.123456Z" {
			t.Fatalf("incorrect restored business row %s %v %s", id, row, version)
		}
		if id == "2" && (*row["creator"] != accountID(t, publisher) || *row["created_at"] == "2020-01-02 03:04:05.123456") {
			t.Fatal("old creation audit copied")
		}
		if *row["modified_at"] == "2020-02-03 04:05:06.123456" {
			t.Fatal("old modify time copied")
		}
	}
	if result.Rollback.PublisherID != accountID(t, publisher) || result.Rollback.ExecutedAt == original.Publication.ExecutedAt {
		t.Fatal("rollback publisher/time lost")
	}
}

func TestReleaseRollbackRestoresStoredTimeDuration(t *testing.T) {
	app, _ := batchEdgeApplication(t, `CREATE TABLE rollback_duration(id bigint PRIMARY KEY,label varchar(40) NOT NULL,duration TIME(6) NOT NULL) ENGINE=InnoDB`, `INSERT INTO rollback_duration VALUES(1,'historical','-120:30:40.123456')`)
	enableMutationPolicy(t, app, "rollback_duration", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"恢复时间","table_name":"rollback_duration","items":[{"operation":"DELETE","id":"1","expected_record_version":"0","content":{}}]}`, "duration")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "duration-publish"), 200)
	quickRestoreFixture(t, app, path, "duration-restore")
	row, version := recordVersionRow(t, app, "rollback_duration", "1")
	if *row["duration"] != "-120:30:40.123456" || version != "2" {
		t.Fatal("historical duration changed", row, version)
	}
}

func mustJSONHistory(history []domain.ReleaseEvent) []byte {
	encoded, _ := json.Marshal(history)
	return encoded
}
