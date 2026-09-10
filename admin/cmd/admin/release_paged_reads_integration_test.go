//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"reflect"
	"testing"
)

// #81 AC-015: normal header reads never materialize the complete application;
// independently fetched pages are bound to the observed whole-order version.
func TestReleaseHeaderAndDetailPagesRequireOneWholeOrderVersion(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	items := make([]any, 21)
	for index := range items {
		items[index] = map[string]any{"table_name": "mutation_add_items", "operation": "ADD", "content": map[string]string{"code": fmt.Sprintf("paged-%02d", index), "label": fmt.Sprintf("proposal %02d", index)}}
	}
	body, _ := json.Marshal(map[string]any{"title": "分页一致性", "items": items})
	saved := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), "paged-create"), 201)
	path := "/api/v1/release-orders/" + saved.ID
	session := integrationAdminSession(t, app)
	rawGet := func(path string) map[string]json.RawMessage {
		response := accountRequest(app, "GET", path, "", session.Result().Cookies(), "")
		if response.Code != 200 {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body)
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	header := rawGet(path)
	if header["items"] != nil || string(header["item_count"]) != "21" {
		t.Fatalf("header loaded whole detail/result document: %s", header)
	}
	first := rawGet(path + "/details?expected_version=1&offset=0&limit=20")
	var pageItems []map[string]json.RawMessage
	if json.Unmarshal(first["items"], &pageItems) != nil || len(pageItems) != 20 || string(first["version"]) != `"1"` || string(first["next_offset"]) != "20" {
		t.Fatalf("first page: %s", first)
	}
	last := rawGet(path + "/details?expected_version=1&offset=20&limit=20")
	if json.Unmarshal(last["items"], &pageItems) != nil || len(pageItems) != 1 || string(pageItems[0]["detail_id"]) != fmt.Sprintf("%q", saved.Items[20].DetailID) {
		t.Fatalf("last page: %s", last)
	}
	rollbackOrderResponse(t, releaseRequest(t, app, "PUT", path, `{"title":"并发改标题","expected_version":"1","changes":{}}`, "paged-change"), 200)
	stale := accountRequest(app, "GET", path+"/details?expected_version=1&offset=20&limit=20", "", session.Result().Cookies(), "")
	assertIntegrationErrorCode(t, stale, 409, "release_version_conflict")
	fresh := rawGet(path)
	if string(fresh["version"]) != `"2"` {
		t.Fatal("header was not current", fresh)
	}
	rawGet(path + "/details?expected_version=2&offset=20&limit=20")
	for _, query := range []string{"offset=0&limit=20", "expected_version=2&offset=0&limit=1001", "expected_version=2&offset=-1&limit=20"} {
		assertIntegrationErrorCode(t, accountRequest(app, "GET", path+"/details?"+query, "", session.Result().Cookies(), ""), 422, "release_invalid")
	}
}

func TestPagedExecutionResultsStayOnOriginalDetailsAndCurrentOrder(t *testing.T) {
	app, db := batchEdgeApplication(t)
	var zone, systemZone, utcNow string
	if err := db.QueryRow(`SELECT @@time_zone,@@system_time_zone,UTC_TIMESTAMP(6)`).Scan(&zone, &systemZone, &utcNow); err != nil {
		t.Fatal(err)
	}
	t.Logf("MySQL time_zone=%s system_time_zone=%s UTC_TIMESTAMP=%s", zone, systemZone, utcNow)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	items := make([]any, 21)
	for index := range items {
		items[index] = map[string]any{"table_name": "mutation_add_items", "operation": "ADD", "content": map[string]string{"code": fmt.Sprintf("actual-page-%02d", index), "label": fmt.Sprintf("original %02d", index)}}
	}
	items[20] = map[string]any{"table_name": "mutation_supplied_id_items", "operation": "ADD", "content": map[string]string{"id": "final-page", "label": "second table"}}
	input, _ := json.Marshal(map[string]any{"title": "跨页多表执行事实", "items": items})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), string(input), "paged-results")
	actor := registerAccount(t, app, "paged.publisher", "paged.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "paged-publisher")
	published := rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/execute", `{"expected_version":"3"}`, "paged-publication"), 200)
	readHeader := func() domain.ReleaseHeader {
		response := releaseActorRequest(t, app, actor, "GET", path, "", "")
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body)
		}
		var shape map[string]json.RawMessage
		var header domain.ReleaseHeader
		if json.Unmarshal(response.Body.Bytes(), &shape) != nil || json.Unmarshal(response.Body.Bytes(), &header) != nil {
			t.Fatal(response.Body)
		}
		if shape["items"] != nil || shape["publication"] != nil || shape["rollback"] != nil {
			t.Fatal("whole result escaped header", response.Body)
		}
		for _, execution := range header.Executions {
			if len(execution.TableVersions) != 2 || execution.ItemCount != 21 {
				t.Fatal(execution)
			}
		}
		return header
	}
	readPage := func(version string, offset int) domain.ReleaseDetailPage {
		response := releaseActorRequest(t, app, actor, "GET", fmt.Sprintf("%s/details?expected_version=%s&offset=%d&limit=20", path, version, offset), "", "")
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body)
		}
		var page domain.ReleaseDetailPage
		if json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatal(response.Body)
		}
		return page
	}
	header := readHeader()
	if header.State != "SUCCEEDED" || len(header.Executions) != 1 {
		t.Fatal(header)
	}
	first, last := readPage("4", 0), readPage("4", 20)
	if len(first.Items) != 20 || len(last.Items) != 1 || last.NextOffset != nil {
		t.Fatal(first, last)
	}
	for index, item := range append(first.Items, last.Items...) {
		if item.DetailID != published.Items[index].DetailID || item.Publication == nil || !reflect.DeepEqual(item.Publication, published.Items[index].Publication) || item.Rollback != nil {
			t.Fatal("publication moved from original detail", index, item)
		}
	}
	// Same-table, same-operation results from one execution must still remain
	// bound to their original stable detail, including server-assigned ADD IDs.
	corruptResult := func(t *testing.T, column, version string, swap bool) {
		t.Helper()
		var rows [2][]byte
		for position := range rows {
			if err := db.QueryRow("SELECT "+column+" FROM rcc_release_details WHERE order_id=? AND position=?", published.ID, position).Scan(&rows[position]); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() {
			for position, encoded := range rows {
				if _, err := db.Exec("UPDATE rcc_release_details SET "+column+"=? WHERE order_id=? AND position=?", encoded, published.ID, position); err != nil {
					t.Error(err)
				}
			}
		})
		if swap {
			for position := range rows {
				if _, err := db.Exec("UPDATE rcc_release_details SET "+column+"=? WHERE order_id=? AND position=?", rows[1-position], published.ID, position); err != nil {
					t.Fatal(err)
				}
			}
		} else {
			if _, err := db.Exec("UPDATE rcc_release_details SET "+column+"=JSON_REMOVE("+column+",'$.detail_id') WHERE order_id=? AND position=0", published.ID); err != nil {
				t.Fatal(err)
			}
		}
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "GET", path+"/details?expected_version="+version+"&offset=0&limit=20", "", ""), 503, "release_unavailable")
	}
	t.Run("reject exchanged publication results on same table", func(t *testing.T) { corruptResult(t, "publication", "4", true) })
	t.Run("reject publication without original detail identity", func(t *testing.T) { corruptResult(t, "publication", "4", false) })
	preview := readQuickPreview(t, app, actor, path, "4")
	restored := rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, ""), "paged-rollback"), 200)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "GET", path+"/details?expected_version=4&offset=20&limit=20", "", ""), 409, "release_version_conflict")
	header = readHeader()
	if header.State != "ROLLED_BACK" || len(header.Executions) != 2 {
		t.Fatal(header)
	}
	first, last = readPage("5", 0), readPage("5", 20)
	for index, item := range append(first.Items, last.Items...) {
		if item.DetailID != published.Items[index].DetailID || !reflect.DeepEqual(item.Publication, published.Items[index].Publication) || item.Rollback == nil || item.Rollback.ID != item.Publication.ID || item.Rollback.ExecutionID != restored.Executions[1].ID || item.Rollback.Operation != "DELETE" {
			t.Fatal("restoration detached from original detail", index, item)
		}
	}
	t.Run("reject rollback without original detail identity", func(t *testing.T) { corruptResult(t, "rollback", "5", false) })
	replay := rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/execute", `{"expected_version":"3"}`, "paged-publication"), 200)
	if replay.State != "SUCCEEDED" || len(replay.Executions) != 1 || replay.Items[20].Rollback != nil {
		t.Fatal("original acknowledgement changed", replay)
	}
	if current := readHeader(); current.State != "ROLLED_BACK" || current.Version != "5" {
		t.Fatal("old acknowledgement replaced current terminal state", current)
	}
	grantReleaseRole(t, app, actor, `["VIEWER"]`, "2", "paged-revoke-publisher")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/execute", `{"expected_version":"3"}`, "paged-publication"), 403, "permission_denied")
	if current := readHeader(); current.State != "ROLLED_BACK" || current.Version != "5" {
		t.Fatal(current)
	}
	readPage("5", 20)
	// The last page rejects an otherwise valid result from the wrong detail.
	var encoded []byte
	if err := db.QueryRow(`SELECT rollback FROM rcc_release_details WHERE order_id=? AND position=20`, published.ID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, db, `UPDATE rcc_release_details SET rollback=JSON_SET(rollback,'$.execution_id','another:ROLLBACK') WHERE order_id='`+published.ID+`' AND position=20`)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "GET", path+"/details?expected_version=5&offset=20&limit=20", "", ""), 503, "release_unavailable")
	if _, err := db.Exec(`UPDATE rcc_release_details SET rollback=? WHERE order_id=? AND position=20`, encoded, published.ID); err != nil {
		t.Fatal(err)
	}
	readPage("5", 20)
}
