//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// AC-037: one order freezes, approves and publishes a mixed set together.
func TestReleaseMixedBatchPublication(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	owner := deliveryDB(t, driver)
	if _, err := owner.Exec(`INSERT INTO mutation_add_items(id,code,label) VALUES(10,'modify','old'),(20,'delete','old')`); err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"items":[{"content":{"code":"new","label":"added"},"operation":"ADD","table_name":"mutation_add_items"},{"content":{"label":"modified"},"expected_record_version":"0","id":"10","operation":"MODIFY","table_name":"mutation_add_items"},{"content":{},"expected_record_version":"0","id":"20","operation":"DELETE","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "mixed")
	response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "mixed-execute")
	if response.Code != 200 {
		t.Fatalf("execute: %d %s", response.Code, response.Body)
	}
	var result domain.ReleaseOrder
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "SUCCEEDED" || len(result.Executions) < 1 || len(executionCommands(result, "PUBLICATION")) != 3 || singleExecutionTableVersion(result.Executions[0]) != "1" {
		t.Fatal(response.Body)
	}
	commands := executionCommands(result, "PUBLICATION")
	if commands[0].ID != "21" || commands[1].ID != "10" || commands[2].ID != "20" || !commands[2].Final.Deleted {
		t.Fatal(response.Body)
	}
	for i, command := range commands {
		if command.RecordVersion != "1" {
			t.Fatalf("item %d version %s", i, command.RecordVersion)
		}
	}
	row, version := recordVersionRow(t, app, "mutation_add_items", "10")
	if version != "1" || *row["label"] != "modified" {
		t.Fatalf("row %v version %s", row, version)
	}
	replay := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "mixed-execute")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatalf("retry: %d %s", replay.Code, replay.Body)
	}
	var count, targets, notifications int
	for query, dest := range map[string]*int{`SELECT COUNT(*) FROM mutation_add_items`: &count, `SELECT COUNT(*) FROM rcc_release_targets`: &targets, `SELECT COUNT(*) FROM rcc_refresh_notifications`: &notifications} {
		if err := owner.QueryRow(query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if count != 2 || targets != 3 || notifications != 1 {
		t.Fatalf("rows %d targets %d notifications %d", count, targets, notifications)
	}
	list := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders?limit=100", "", "")
	if list.Code != 200 || strings.Contains(list.Body.String(), `"items"`) || strings.Contains(list.Body.String(), `"publication"`) || !strings.Contains(list.Body.String(), `"item_count":3`) {
		t.Fatalf("list must provide bounded summaries: %d, bytes %d", list.Code, list.Body.Len())
	}

}

// AC-038: MySQL-equivalent known identities cannot be repeated or partly saved.
func TestReleaseBatchDuplicateIdentityDoesNotReplaceDraft(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"kept"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`, "duplicate-base")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var original domain.ReleaseOrder
	json.Unmarshal(created.Body.Bytes(), &original)
	path := "/api/v1/release-orders/" + original.ID
	update := releaseRequest(t, app, "PUT", path, `{"expected_version":"1","items":[{"content":{"code":"discarded"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"},{"content":{},"expected_record_version":"0","id":"01","operation":"DELETE","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`, "duplicate-edit")
	assertIntegrationErrorCode(t, update, 422, "release_duplicate_target")
	var issue struct {
		Error struct {
			ItemIndex *int `json:"item_index"`
		}
	}
	json.Unmarshal(update.Body.Bytes(), &issue)
	if issue.Error.ItemIndex == nil || *issue.Error.ItemIndex != 1 {
		t.Fatalf("missing item position: %s", update.Body)
	}
	current := releaseReadAllDetails(t, app, "GET", path, "", "")
	if current.Body.String() != created.Body.String() {
		t.Fatalf("partial replacement: %s", current.Body)
	}
}

// AC-037/040: the real executable uses deployment socket/HTTP/transaction
// budgets, and an original-key retry returns all 1,000 actual results.
func TestReleaseThousandItemsThroughExecutable(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	db := deliveryDB(t, driver)
	values := []string{}
	for i := 1; i <= 666; i++ {
		values = append(values, fmt.Sprintf("(%d,'seed-%d','old')", i, i))
	}
	if _, err := db.Exec("INSERT INTO mutation_add_items(id,code,label) VALUES" + strings.Join(values, ",")); err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := registerAccount(t, app, "batch.reviewer", "batch.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "batch-reviewer-role")
	process := accountProcessCommand(t, buildIntegrationAdmin(t), driver)
	process.ready(t)
	cookies, csrf, _ := processCredentials(t, process, "/api/v1/auth/login", `{"username":"integration.user","password":"correct horse battery staple"}`)
	reviewCookies, reviewCSRF, _ := processCredentials(t, process, "/api/v1/auth/login", `{"username":"batch.reviewer","password":"correct horse battery staple"}`)
	items := []any{}
	for i := 0; i < 1000; i++ {
		item := map[string]any{"table_name": "mutation_add_items", "operation": "ADD", "content": map[string]string{"code": fmt.Sprintf("added-%d", i), "label": "new"}}
		if i < 666 {
			item["id"] = strconv.Itoa(i + 1)
			item["expected_record_version"] = "0"
			if i < 333 {
				item["operation"] = "MODIFY"
				item["content"] = map[string]string{"label": "modified"}
			} else {
				item["operation"] = "DELETE"
				item["content"] = map[string]string{}
			}
		}
		items = append(items, item)
	}
	input, _ := json.Marshal(map[string]any{"title": "集成测试发布单", "items": items})
	request := func(path, body, key string, actorCookies []*http.Cookie, actorCSRF string) []byte {
		t.Helper()
		req, err := http.NewRequest("POST", process.origin+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", process.publicOrigin)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", actorCSRF)
		req.Header.Set("Idempotency-Key", key)
		for _, cookie := range actorCookies {
			req.AddCookie(cookie)
		}
		start := time.Now()
		response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer response.Body.Close()
		result, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: status %d, request %d bytes, response %d bytes, elapsed %s", path, response.StatusCode, len(body), len(result), time.Since(start))
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("%s: %d %s", path, response.StatusCode, result)
		}
		return result
	}
	created := request("/api/v1/release-orders", string(input), "thousand-create", cookies, csrf)
	var order domain.ReleaseOrder
	if json.Unmarshal(created, &order) != nil {
		t.Fatal("invalid create")
	}
	path := "/api/v1/release-orders/" + order.ID
	request(path+"/submit", `{"expected_version":"1"}`, "thousand-submit", cookies, csrf)
	request(path+"/approve", `{"expected_version":"2","reason":"reviewed all 1000 items"}`, "thousand-approve", reviewCookies, reviewCSRF)
	result := request(path+"/execute", `{"expected_version":"3"}`, "thousand-execute", cookies, csrf)
	if json.Unmarshal(result, &order) != nil || order.State != "SUCCEEDED" || len(order.Executions) < 1 || len(executionCommands(order, "PUBLICATION")) != 1000 {
		t.Fatal("incomplete result")
	}
	replay := request(path+"/execute", `{"expected_version":"3"}`, "thousand-execute", cookies, csrf)
	if string(replay) != string(result) {
		t.Fatal("original-key result changed")
	}
	seen := map[string]bool{}
	for i, command := range executionCommands(order, "PUBLICATION") {
		if seen[command.ID] || command.RecordVersion != "1" || command.Sequence != strconv.Itoa(i+1) {
			t.Fatalf("invalid item %d %+v", i, command)
		}
		seen[command.ID] = true
		if i < 666 && command.ID != strconv.Itoa(i+1) {
			t.Fatalf("wrong existing identity %d %s", i, command.ID)
		}
		if i >= 666 && command.ID != strconv.Itoa(i+1) {
			t.Fatalf("wrong actual new identity %d %s", i, command.ID)
		}
	}
	var count, commands, versions, notifications, targets int
	for query, dest := range map[string]*int{`SELECT COUNT(*) FROM mutation_add_items`: &count, `SELECT COUNT(*) FROM rcc_publication_commands`: &commands, `SELECT COUNT(*) FROM rcc_record_versions WHERE lock_version=1`: &versions, `SELECT COUNT(*) FROM rcc_refresh_notifications`: &notifications, `SELECT COUNT(*) FROM rcc_release_targets`: &targets} {
		if err := db.QueryRow(query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if count != 667 || commands != 1000 || versions != 1000 || notifications != 1 || targets != 1000 {
		t.Fatalf("partial/duplicate state %d %d %d %d %d", count, commands, versions, notifications, targets)
	}
	// AC-041: exercise the complete reverse under the same executable's default
	// four-second transaction budget, without repeating the forward fixture.
	previewBytes := request(path+"/quick-rollback/preview", `{"expected_version":"4"}`, "thousand-preview", cookies, csrf)
	var preview quickPreviewResponse
	if json.Unmarshal(previewBytes, &preview) != nil || len(preview.Items) != 1000 {
		t.Fatal("incomplete restoration preview")
	}
	body := quickRollbackBody("4", preview.Digest, "")
	restored := request(path+"/quick-rollback", body, "thousand-restore", cookies, csrf)
	var reverse domain.ReleaseOrder
	if json.Unmarshal(restored, &reverse) != nil || reverse.State != "ROLLED_BACK" || reverse.ID != order.ID || singleExecutionTableVersion(reverse.Executions[1]) != "2" || len(executionCommands(reverse, "ROLLBACK")) != 1000 {
		t.Fatal("incomplete original rollback")
	}
	if string(request(path+"/quick-rollback", body, "thousand-restore", cookies, csrf)) != string(restored) {
		t.Fatal("rollback original-key result changed")
	}

	for index, command := range executionCommands(reverse, "ROLLBACK") {
		sourceIndex := len(executionCommands(order, "PUBLICATION")) - 1 - index
		if command.ID != executionCommands(order, "PUBLICATION")[sourceIndex].ID || command.RecordVersion != "2" || command.Final.Deleted != (sourceIndex >= 666) {
			t.Fatalf("incorrect inverse %d", index)
		}
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM mutation_add_items`: 666, `SELECT COUNT(*) FROM mutation_add_items WHERE id<=666 AND label='old'`: 666, `SELECT COUNT(*) FROM rcc_record_versions WHERE lock_version=2`: 1000, `SELECT COUNT(*) FROM rcc_publication_commands`: 2000, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 2, `SELECT COUNT(*) FROM rcc_release_targets`: 0})
	current := rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
	if current.State != "ROLLED_BACK" || current.ID != reverse.ID {
		t.Fatal("thousand inverse missing original association")
	}
}

func TestReleaseBatchLargeFieldDraftPreservesInput(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	input, _ := json.Marshal(map[string]any{"title": "集成测试发布单", "items": []any{map[string]any{"table_name": "mutation_add_items", "operation": "ADD", "content": map[string]string{"code": "oversize", "label": strings.Repeat("x", 65537)}}}})
	response := releaseRequest(t, app, "POST", "/api/v1/release-orders", string(input), "field-budget")
	if response.Code != 201 {
		t.Fatalf("large field draft: %d %.500s", response.Code, response.Body.String())
	}
	var order domain.ReleaseOrder
	if json.Unmarshal(response.Body.Bytes(), &order) != nil || len(*order.Items[0].Content["label"]) != 65537 {
		t.Fatal("large draft field truncated")
	}
}

// Database defaults may expand a small request past former JSON budgets.
// Every actual value and exact replay remains intact in the same transaction.
func TestReleaseBatchExpandedResultsExceedFormerBudget(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	rootDriver := *driver
	rootDriver.User = "root"
	db := deliveryDB(t, &rootDriver)
	if _, err := db.Exec(`CREATE TABLE batch_large_result(id bigint unsigned AUTO_INCREMENT PRIMARY KEY,code varchar(32) NOT NULL,payload LONGTEXT NOT NULL DEFAULT (REPEAT('x',65536))) ENGINE=InnoDB`); err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "batch_large_result", mutationPolicyFixture{AllowAdd: true})
	items := []any{}
	for i := 0; i < 130; i++ {
		items = append(items, map[string]any{"table_name": "batch_large_result", "operation": "ADD", "content": map[string]string{"code": fmt.Sprint(i)}})
	}
	input, _ := json.Marshal(map[string]any{"title": "集成测试发布单", "items": items})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), string(input), "expanded")
	result := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "expanded-execute")
	if result.Code != 200 || result.Body.Len() <= 8<<20 {
		t.Fatalf("expanded result status %d, response bytes %d, %.500s", result.Code, result.Body.Len(), result.Body.String())
	}
	var order domain.ReleaseOrder
	if json.Unmarshal(result.Body.Bytes(), &order) != nil || len(executionCommands(order, "PUBLICATION")) != 130 {
		t.Fatal("missing expanded results")
	}
	for _, command := range executionCommands(order, "PUBLICATION") {
		for _, field := range command.Final.Fields {
			if field.Name == "payload" && (field.Value == nil || *field.Value != strings.Repeat("x", 65536)) {
				t.Fatal("expanded payload truncated")
			}
		}
	}
	var rows, commands, versions, requests int
	for query, dest := range map[string]*int{`SELECT COUNT(*) FROM batch_large_result`: &rows, `SELECT COUNT(*) FROM rcc_publication_commands`: &commands, `SELECT COUNT(*) FROM rcc_record_versions`: &versions, `SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'execute:%'`: &requests} {
		if err := db.QueryRow(query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if rows != 130 || commands != 130 || versions != 130 || requests != 1 {
		t.Fatalf("partial budget effects %d %d %d %d", rows, commands, versions, requests)
	}
	current := releaseReadAllDetails(t, app, "GET", path, "", "")
	if !strings.Contains(current.Body.String(), `"state":"SUCCEEDED"`) {
		t.Fatal("approval lost")
	}
}

// Long-lived history is a persistence fixture; approval and cancellation still
// use the public API. Large workflow history must not strand active targets.
func TestReleaseBatchLargeHistoryCanApproveAndCancel(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	reviewer := publicationFixtureReviewer(t, app)
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"headroom"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`, "headroom-create")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var order domain.ReleaseOrder
	json.Unmarshal(created.Body.Bytes(), &order)
	path := "/api/v1/release-orders/" + order.ID
	submitted := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "headroom-submit")
	if submitted.Code != 200 {
		t.Fatal(submitted.Body)
	}
	db := deliveryDB(t, driver)
	var document []byte
	if err := db.QueryRow(`SELECT document FROM rcc_release_orders WHERE id=?`, order.ID).Scan(&document); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(document, &order)
	// A long-lived order fixture has sequential EDIT history followed by its
	// real SUBMIT. Public actions below still enforce versions and target locks.
	submittedEvent := order.History[1]
	order.History = order.History[:1]
	initial, _ := json.Marshal(order)
	used := len(initial)
	next := 2
	at, err := time.Parse(time.RFC3339Nano, order.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	for used < 8<<20-64<<10-900 {
		event := domain.ReleaseEvent{Action: "EDIT", ActorID: order.ApplicantID, Version: strconv.Itoa(next), At: at.Add(time.Duration(next) * time.Second).Format(time.RFC3339Nano)}
		encodedEvent, _ := json.Marshal(event)
		used += len(encodedEvent) + 1
		order.History = append(order.History, event)
		next++
	}
	order.Version = strconv.Itoa(next)
	submittedEvent.Version = order.Version
	submittedEvent.At = at.Add(time.Duration(next) * time.Second).Format(time.RFC3339Nano)
	order.History = append(order.History, submittedEvent)
	order.UpdatedAt = submittedEvent.At
	encoded, _ := json.Marshal(order)
	if len(encoded) > 8<<20-64<<10 || len(encoded) < 8<<20-64<<10-1000 {
		t.Fatalf("fixture bytes %d", len(encoded))
	}
	seedReleaseWorkflowHistory(t, db, order)
	approval := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", `{"expected_version":"`+order.Version+`","reason":"`+strings.Repeat("<", 2000)+`"}`, "headroom-approve")
	if approval.Code != 200 {
		t.Fatalf("large-history approval: status %d bytes %d", approval.Code, approval.Body.Len())
	}
	approvedVersion := strconv.Itoa(next + 1)
	cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"`+approvedVersion+`","reason":"`+strings.Repeat("<", 2000)+`"}`, "headroom-cancel")
	if cancelled.Code != 200 {
		t.Fatalf("cancellation: %d bytes %d", cancelled.Code, cancelled.Body.Len())
	}
	var targets int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_release_targets`).Scan(&targets); err != nil || targets != 0 {
		t.Fatalf("stranded targets %d %v", targets, err)
	}
}
