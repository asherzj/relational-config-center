//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type quickPreviewResponse struct {
	OrderID         string               `json:"order_id"`
	ExpectedVersion string               `json:"expected_version"`
	TableName       string               `json:"table_name"`
	Digest          string               `json:"preview_digest"`
	Items           []domain.ReleaseItem `json:"items"`
}

// #59 AC-004/015: read the complete actual restoration before confirming it.
func TestQuickRollbackPreviewShowsWholeRestorationWithoutWriting(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'before-modify','old'),(20,'before-delete','retained')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"渠道紧急修正","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"new","label":"added"}},{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}},{"operation":"DELETE","id":"20","expected_record_version":"0","content":{}}]}`, "quick-preview")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-preview-publish"), 200)
	publisher := registerAccount(t, app, "quick.publisher", "quick.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "quick-publisher-role")
	response := releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "")
	if response.Code != 200 {
		t.Fatalf("preview: %d %s", response.Code, response.Body)
	}
	var preview quickPreviewResponse
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.OrderID != original.ID || preview.ExpectedVersion != "4" || preview.TableName != "mutation_add_items" || len(preview.Digest) != 64 || len(preview.Items) != 3 {
		t.Fatal("preview did not identify the complete reviewed version", preview)
	}
	if preview.Items[0].Operation != "ADD" || *preview.Items[0].Content["label"] != "retained" || preview.Items[1].Operation != "MODIFY" || *preview.Items[1].Before["label"] != "published" || *preview.Items[1].Content["label"] != "old" || preview.Items[2].Operation != "DELETE" || string(*preview.Items[2].ID) != original.Publication.Commands[0].ID {
		t.Fatal("restoration was not derived from actual whole publication", preview.Items)
	}
	for _, item := range preview.Items {
		if item.RecordKey != nil || item.RecordTable != "" {
			t.Fatal("private target metadata leaked")
		}
	}
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(original, current) {
		t.Fatal("preview changed order or history")
	}
	row, version := recordVersionRow(t, app, "mutation_add_items", "10")
	if *row["label"] != "published" || version != "1" {
		t.Fatal("preview wrote configuration")
	}
}

// #59 AC-003/004/006: a different current publisher restores once without approval.
func TestQuickRollbackRestoresMixedPublicationAndReplaysActualResult(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'quick-old','old'),(20,'quick-retained','retained')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"渠道配置修正","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"quick-new","label":"added"}},{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}},{"operation":"DELETE","id":"20","expected_record_version":"0","content":{}}]}`, "quick-mixed")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-mixed-publish"), 200)
	publisher := registerAccount(t, app, "quick.executor", "quick.executor@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "quick-executor-role")
	preview := readQuickPreview(t, app, publisher, path, "4")
	body := quickRollbackBody("4", preview.Digest, "恢复整单配置")
	response := releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback", body, "quick-mixed-execute")
	reverse := rollbackOrderResponse(t, response, 200)
	if reverse.State != "ROLLED_BACK" || reverse.ID != original.ID || reverse.Rollback == nil || reverse.Rollback.PublisherID != accountID(t, publisher) || reverse.ApplicantID != original.ApplicantID || reverse.Rollback.TableVersion != "2" {
		t.Fatal("missing original rollback result", reverse)
	}
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(current.Items, original.Items) || !reflect.DeepEqual(current.Publication, original.Publication) || len(current.History) != len(original.History)+1 {
		t.Fatal("original application overwritten")
	}
	event := current.History[len(current.History)-1]
	if event.Action != "QUICK_ROLLBACK" || event.ActorID != accountID(t, publisher) || event.Reason != "恢复整单配置" {
		t.Fatal("rollback history missing", event)
	}
	for _, id := range []string{"10", "20"} {
		row, version := recordVersionRow(t, app, "mutation_add_items", id)
		expected := "old"
		if id == "20" {
			expected = "retained"
		}
		if *row["label"] != expected || version != "2" {
			t.Fatal("mixed restore missed row or version", row, version)
		}
	}
	generated := reverse.Rollback.Commands[2]
	if generated.Operation != "DELETE" || !generated.Final.Deleted || generated.ID != original.Publication.Commands[0].ID || generated.RecordVersion != "2" {
		t.Fatal("generated row was not restored to absence", generated)
	}
	replay := releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback", body, "quick-mixed-execute")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("original request did not replay its exact result", replay.Body)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, "changed reason"), "quick-mixed-execute"), 409, "idempotency_conflict")
	for _, action := range []string{"complete", "rollback", "quick-rollback"} {
		invalid := `{"expected_version":"5","reason":"no reverse again"}`
		if action == "complete" {
			invalid = `{"expected_version":"5"}`
		}
		if action == "quick-rollback" {
			invalid = quickRollbackBody("5", preview.Digest, "no reverse again")
		}
		assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+reverse.ID+"/"+action, invalid, "quick-terminal-"+action), 422, "release_state_invalid")
	}
	// Every original target, including the generated and deleted identity, is free.
	next := fmt.Sprintf(`{"title":"回滚后新单","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"2","content":{"label":"next"}},{"operation":"MODIFY","id":"20","expected_record_version":"2","content":{"label":"next"}},{"operation":"ADD","expected_record_version":"2","content":{"id":%q,"code":"released-id","label":"next"}}]}`, generated.ID)
	approvePublication(t, app, reviewer, next, "quick-after")
}

func readQuickPreview(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, path, version string) quickPreviewResponse {
	t.Helper()
	response := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback/preview", fmt.Sprintf(`{"expected_version":%q}`, version), "")
	if response.Code != 200 {
		t.Fatalf("preview: %d %s", response.Code, response.Body)
	}
	var result quickPreviewResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func quickRollbackBody(version, digest, reason string) string {
	body, _ := json.Marshal(map[string]string{"expected_version": version, "preview_digest": digest, "reason": reason})
	return string(body)
}

// #59 D-007/AC-005: external SQL may change values without advancing RCC versions.
func TestQuickRollbackRejectsUnversionedExternalChangesBeforePreviewAndExecution(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'external-a','old-a'),(20,'external-b','old-b')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"title":"外部变更保护","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published-a"}},{"operation":"MODIFY","id":"20","expected_record_version":"0","content":{"label":"published-b"}}]}`, "quick-external")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-external-publish"), 200)
	actor := registerAccount(t, app, "quick.external", "quick.external@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "quick-external-role")
	preview := readQuickPreview(t, app, actor, path, "4")
	deliveryExec(t, db, `UPDATE mutation_add_items SET label='external-newer' WHERE id=10`)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, ""), 409, "record_version_conflict")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, "restore"), "quick-external-execute"), 409, "record_version_conflict")
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	assertReleaseFailureOnly(t, original, current, "QUICK_ROLLBACK_FAILED")
	for _, id := range []string{"10", "20"} {
		row, version := recordVersionRow(t, app, "mutation_add_items", id)
		expected := "external-newer"
		if id == "20" {
			expected = "published-b"
		}
		if *row["label"] != expected || version != "1" {
			t.Fatal("failed restoration partially applied", row, version)
		}
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"占用验证","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"1","content":{"label":"next"}},{"operation":"MODIFY","id":"20","expected_record_version":"1","content":{"label":"next"}}]}`, "quick-external-conflict"), 409, "release_target_conflict")
}

// Fault at the storage boundary: a quick restoration must use its original's
// retained target, never treat a missing or foreign reservation as permission.
func TestQuickRollbackRequiresItsOriginalRetainedTargets(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'target-a','old'),(20,'target-b','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"原目标保护","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}},{"operation":"MODIFY","id":"20","expected_record_version":"0","content":{"label":"published"}}]}`, "quick-target")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-target-publish"), 200)
	actor := registerAccount(t, app, "quick.target", "quick.target@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "quick-target-role")
	preview := readQuickPreview(t, app, actor, path, "4")
	deliveryExec(t, db, `DELETE FROM rcc_release_targets WHERE order_id=? LIMIT 1`, original.ID)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, "restore"), "quick-target-execute"), 409, "release_target_conflict")
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	assertReleaseFailureOnly(t, original, current, "QUICK_ROLLBACK_FAILED")
	for _, id := range []string{"10", "20"} {
		row, version := recordVersionRow(t, app, "mutation_add_items", id)
		if *row["label"] != "published" || version != "1" {
			t.Fatal("target failure partially restored", row, version)
		}
	}
}

func TestQuickRollbackUsesCurrentRolesAndRequiresReviewedVersionDigestAndCurrentRole(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'quick-role','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"权限与审阅","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}}]}`, "quick-role")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-role-publish"), 200)
	actor := registerAccount(t, app, "quick.roles", "quick.roles@example.com", "correct horse battery staple")
	for i, roles := range []string{`["VIEWER"]`, `["EDITOR"]`, `["APPROVER"]`} {
		if i > 0 {
			grantReleaseRole(t, app, actor, roles, fmt.Sprint(i), fmt.Sprintf("quick-role-%d", i))
		}
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, ""), 403, "permission_denied")
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", strings.Repeat("a", 64), "restore"), "quick-role-deny"), 403, "permission_denied")
		detail := releaseActorRequest(t, app, actor, "GET", path, "", "")
		if strings.Contains(detail.Body.String(), `"quick-rollback"`) {
			t.Fatal("quick rollback advertised to unauthorized role")
		}
	}
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "3", "quick-role-publisher")
	preview := readQuickPreview(t, app, actor, path, "4")
	for i, body := range []string{quickRollbackBody("4", "", "reason"), quickRollbackBody("4", preview.Digest, strings.Repeat("界", 667)), `{"expected_version":"4","reason":"no preview"}`} {
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, fmt.Sprintf("quick-role-invalid-%d", i)), 422, "release_invalid")
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("3", preview.Digest, "stale version"), "quick-role-stale"), 409, "release_version_conflict")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", strings.Repeat("a", 64), "unreviewed"), "quick-role-digest"), 409, "release_frozen_changed")
	body := quickRollbackBody("4", preview.Digest, "reviewed")
	grantReleaseRole(t, app, actor, `["VIEWER"]`, "4", "quick-role-revoke")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "quick-role-execute"), 403, "permission_denied")
	grantReleaseRole(t, app, actor, `["ADMIN"]`, "5", "quick-role-admin")
	result := rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "quick-role-execute"), 200)
	if result.Rollback.PublisherID != accountID(t, actor) {
		t.Fatal("current administrator was not recorded")
	}
	grantReleaseRole(t, app, actor, `["VIEWER"]`, "6", "quick-role-revoke-replay")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "quick-role-execute"), 403, "permission_denied")
}

// #59 AC-006: one observed version admits one terminal business result.
func TestQuickRollbackCompetesWithCompletionAndOtherRollbacks(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	reviewer := publicationFixtureReviewer(t, app)
	actor := registerAccount(t, app, "quick.races", "quick.races@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "quick-race-role")
	for i, mode := range []string{"complete", "quick-distinct-key", "quick-same-key"} {
		t.Run(mode, func(t *testing.T) {
			id := fmt.Sprint(100 + i)
			deliveryExec(t, db, `INSERT INTO mutation_add_items(id,code,label) VALUES(?,?,'old')`, id, "race-"+id)
			path := approvePublication(t, app, reviewer, fmt.Sprintf(`{"title":"竞争终止","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":%q,"expected_record_version":"0","content":{"label":"published"}}]}`, id), "quick-race-"+mode)
			rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-race-publish-"+mode), 200)
			preview := readQuickPreview(t, app, actor, path, "4")
			body := quickRollbackBody("4", preview.Digest, "restore once")
			type outcome struct {
				action, key, body string
				response          *httptest.ResponseRecorder
			}
			outcomes := make(chan outcome, 2)
			start := make(chan struct{})
			cookies, csrf := actor.Result().Cookies(), sessionCSRF(t, actor)
			for n := 0; n < 2; n++ {
				action, key, request := "quick-rollback", fmt.Sprintf("quick-race-%s-%d", mode, n), body
				if n == 1 && mode == "complete" {
					action, request = "complete", `{"expected_version":"4"}`
				}
				if n == 1 && mode == "quick-same-key" {
					key = "quick-race-" + mode + "-0"
				}
				go func(action, key, request string) {
					<-start
					outcomes <- outcome{action, key, request, accountRequestFrom(app, "POST", path+"/"+action, request, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})}
				}(action, key, request)
			}
			close(start)
			successes := []outcome{}
			for range 2 {
				out := <-outcomes
				if out.response.Code == 200 {
					successes = append(successes, out)
				} else {
					assertIntegrationErrorCode(t, out.response, 409, "release_version_conflict")
				}
			}
			expected := 1
			if mode == "quick-same-key" {
				expected = 2
			}
			if len(successes) != expected {
				t.Fatal("wrong number of successful requests", len(successes))
			}
			if expected == 2 && successes[0].response.Body.String() != successes[1].response.Body.String() {
				t.Fatal("same key diverged")
			}
			winner := successes[0]
			replay := releaseActorRequest(t, app, actor, "POST", path+"/"+winner.action, winner.body, winner.key)
			if replay.Code != 200 || replay.Body.String() != winner.response.Body.String() {
				t.Fatal("winner result did not replay exactly")
			}
			original := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
			if original.Version != "5" || len(original.History) != 5 {
				t.Fatal("duplicate terminal workflow result", original)
			}
			row, version := recordVersionRow(t, app, "mutation_add_items", id)
			if winner.action == "complete" {
				if original.State != "COMPLETED" || *row["label"] != "published" || version != "1" {
					t.Fatal("completion race changed data")
				}
			} else {
				if original.State != "ROLLED_BACK" || *row["label"] != "old" || version != "2" {
					t.Fatal("rollback race did not restore once")
				}
			}
		})
	}
}

// #59 AC-005: real MySQL persistence errors after DML never leave partial results.
func TestQuickRollbackPersistenceFailuresPreserveValuesVersionsHistoryAndTargets(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'fault-a','old-a'),(20,'fault-b','old-b')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"原子恢复","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}},{"operation":"MODIFY","id":"20","expected_record_version":"0","content":{"label":"published"}}]}`, "quick-fault")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-fault-publish"), 200)
	actor := registerAccount(t, app, "quick.fault", "quick.fault@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "quick-fault-role")
	preview := readQuickPreview(t, app, actor, path, "4")
	body := quickRollbackBody("4", preview.Digest, "restore atomically")
	conflictBody := `{"title":"目标仍保护","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"1","content":{"label":"next"}},{"operation":"MODIFY","id":"20","expected_record_version":"1","content":{"label":"next"}}]}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", conflictBody, "quick-fault-conflict"), 409, "release_target_conflict")

	for _, failure := range []struct{ table, event, condition string }{
		{"rcc_record_versions", "UPDATE", "NEW.lock_version>1"},
		{"rcc_publication_commands", "INSERT", "TRUE"},
		{"rcc_table_publications", "UPDATE", "NEW.table_version>1"},
		{"rcc_refresh_notifications", "INSERT", "TRUE"},
		{"rcc_release_details", "UPDATE", "NEW.rollback IS NOT NULL"},
		{"rcc_release_executions", "INSERT", "NEW.kind='ROLLBACK'"},
		{"rcc_release_orders", "UPDATE", "NEW.state='ROLLED_BACK'"},
		{"rcc_release_targets", "DELETE", "TRUE"},
		{"rcc_release_requests", "UPDATE", "NEW.result IS NOT NULL"},
	} {
		t.Run(failure.table+"_"+failure.event, func(t *testing.T) {
			deliveryExec(t, db, fmt.Sprintf("CREATE TRIGGER fail_quick BEFORE %s ON %s FOR EACH ROW BEGIN IF %s THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='quick restoration persistence boundary'; END IF; END", failure.event, failure.table, failure.condition))
			response := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "quick-fault-execute")
			deliveryExec(t, db, `DROP TRIGGER fail_quick`)
			code := "release_unavailable"
			if failure.table == "rcc_record_versions" {
				code = "mutation_unavailable"
			}
			assertIntegrationErrorCode(t, response, 503, code)
			current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
			assertReleaseFailureOnly(t, original, current, "QUICK_ROLLBACK_FAILED")
			original = current
			for _, id := range []string{"10", "20"} {
				row, version := recordVersionRow(t, app, "mutation_add_items", id)
				if *row["label"] != "published" || version != "1" {
					t.Fatal("failed reverse partially changed row", row, version)
				}
			}
			assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", conflictBody, "quick-fault-conflict"), 409, "release_target_conflict")
			list := releaseRequest(t, app, "GET", "/api/v1/release-orders?state=COMPLETED&table_name=mutation_add_items", "", "")
			if list.Code != 200 || !strings.Contains(list.Body.String(), `"orders":[]`) {
				t.Fatal("failed reverse left a result record", list.Body)
			}
		})
	}
	result := rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "quick-fault-execute"), 200)
	if result.Rollback.TableVersion != "2" || result.Rollback.Commands[0].Sequence != "3" || result.Rollback.Commands[1].Sequence != "4" {
		t.Fatal("failed attempts advanced public publication progress", result.Publication)
	}
}

func TestQuickRollbackRejectsChangedSchemaRulesAndRecordVersions(t *testing.T) {
	app, db := batchEdgeApplication(t, `CREATE TABLE quick_semantics(id bigint unsigned PRIMARY KEY,code varchar(32) NOT NULL,label varchar(64) NOT NULL) ENGINE=InnoDB`, `INSERT INTO quick_semantics(id,code,label) VALUES(10,'quick-semantics','old')`)
	enableMutationPolicy(t, app, "quick_semantics", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"执行语义冻结","table_name":"quick_semantics","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}}]}`, "quick-semantics")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-semantics-publish"), 200)
	actor := integrationAdminSession(t, app)
	preview := readQuickPreview(t, app, actor, path, "4")
	reject := func(code string) {
		t.Helper()
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, ""), 409, code)
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, "restore"), "quick-semantics-execute"), 409, code)
		current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
		assertReleaseFailureOnly(t, original, current, "QUICK_ROLLBACK_FAILED")
		original = current
	}
	deliveryExec(t, db, `ALTER TABLE quick_semantics MODIFY label varchar(65) NOT NULL`)
	reject("release_frozen_changed")
	deliveryExec(t, db, `ALTER TABLE quick_semantics MODIFY label varchar(64) NOT NULL`)
	replacePolicyAssignment(t, app, "quick_semantics", queryPolicyFixture{}, mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	reject("release_frozen_changed")
	restored := policyIntegrationRequest(t, app, "PUT", "/api/v1/table-policies/quick_semantics", `{"table_name":"quick_semantics","query_policy_code":"fixture_quick_semantics_query_v1","mutation_policy_code":"fixture_quick_semantics_mutation_v1"}`)
	if restored.Code != 200 {
		t.Fatal(restored.Body)
	}
	// Non-execution display metadata is harmless and leaves the reviewed digest stable.
	renamed := policyIntegrationRequest(t, app, "PATCH", "/api/v1/mutation-policies/fixture_quick_semantics_mutation_v1/metadata", `{"name":"当前显示名称","description":"不改变执行"}`)
	if renamed.Code != 200 {
		t.Fatal(renamed.Body)
	}
	again := readQuickPreview(t, app, actor, path, "4")
	if again.Digest != preview.Digest {
		t.Fatal("display-only metadata changed restoration intent")
	}
	deliveryExec(t, db, `UPDATE rcc_record_versions SET lock_version=lock_version+1 WHERE table_name='quick_semantics'`)
	reject("record_version_conflict")
	row, version := recordVersionRow(t, app, "quick_semantics", "10")
	if *row["label"] != "published" || version != "2" {
		t.Fatal("record version refusal changed business state")
	}
}

func TestQuickRollbackConstraintFailureRollsBackEarlierItems(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'constraint-a','old-a'),(20,'constraint-restore','old-b')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"恢复约束","table_name":"mutation_add_items","items":[{"operation":"DELETE","id":"20","expected_record_version":"0","content":{}},{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}}]}`, "quick-constraint")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-constraint-publish"), 200)
	actor := integrationAdminSession(t, app)
	preview := readQuickPreview(t, app, actor, path, "4")
	// A disjoint external row now owns the deleted row's unique business value.
	deliveryExec(t, db, `INSERT INTO mutation_add_items(id,code,label) VALUES(30,'constraint-restore','external')`)
	response := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, "restore"), "quick-constraint-execute")
	assertIntegrationErrorCode(t, response, 409, "duplicate_key")
	batchEdgeIndex(t, response, 1)
	row, version := recordVersionRow(t, app, "mutation_add_items", "10")
	if *row["label"] != "published" || version != "1" {
		t.Fatal("earlier restored item survived rollback")
	}
	row, version = recordVersionRow(t, app, "mutation_add_items", "30")
	if *row["label"] != "external" || version != "0" {
		t.Fatal("external row changed")
	}
	absent := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/query", `{"conditions":[{"field":"id","operator":"exact","value":"20"}]}`)
	if absent.Code != 200 || !strings.Contains(absent.Body.String(), `"rows":[]`) {
		t.Fatal("failed ADD survived", absent.Body)
	}
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	assertReleaseFailureOnly(t, original, current, "QUICK_ROLLBACK_FAILED")
}

func TestQuickRollbackCanTerminateAtAcceptedPublicationCapacity(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'quick-capacity','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"容量边界恢复","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}}]}`, "quick-capacity")
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-capacity-publish"), 200)
	stored, err := app.mysql.GetReleaseOrder(t.Context(), published.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored = seedLongPublishedHistory(t, db, stored)
	encoded, _ := json.Marshal(stored)
	limit := application.ReleaseResultBytes - application.ReleaseContinuationHeadroom
	if len(encoded) > limit || len(encoded) < limit-2000 {
		t.Fatalf("not at valid publication boundary: %d", len(encoded))
	}
	actor := integrationAdminSession(t, app)
	preview := readQuickPreview(t, app, actor, path, stored.Version)
	body := quickRollbackBody(stored.Version, preview.Digest, strings.Repeat("界", 666))
	response := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "quick-capacity-execute")
	reverse := rollbackOrderResponse(t, response, 200)
	original := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if reverse.State != "ROLLED_BACK" || original.State != "ROLLED_BACK" {
		t.Fatal("legal large publication could not terminate")
	}
	replay := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "quick-capacity-execute")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("capacity result did not recover")
	}
	row, version := recordVersionRow(t, app, "mutation_add_items", "10")
	if *row["label"] != "old" || version != "2" {
		t.Fatal("capacity restoration failed")
	}
}

func TestQuickRollbackRejectsDatabaseEffectsThatCannotRestoreOriginalValues(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label,quantity) VALUES(10,'quick-effect','old',10)`, `CREATE TRIGGER quick_quantity BEFORE UPDATE ON mutation_add_items FOR EACH ROW SET NEW.quantity=NEW.quantity+1`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"恢复效果校验","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}}]}`, "quick-effect")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "quick-effect-publish"), 200)
	actor := integrationAdminSession(t, app)
	preview := readQuickPreview(t, app, actor, path, "4")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, "restore original"), "quick-effect-execute"), 422, "rollback_restore_mismatch")
	row, version := recordVersionRow(t, app, "mutation_add_items", "10")
	if *row["label"] != "published" || *row["quantity"] != "11" || version != "1" {
		t.Fatal("mismatched restore was committed", row, version)
	}
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	assertReleaseFailureOnly(t, original, current, "QUICK_ROLLBACK_FAILED")
}
