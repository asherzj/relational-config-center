//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var publicationFixtureSequence atomic.Uint64
var publicationFixtureReviewers = struct {
	sync.Mutex
	byApp map[*adminApplication]*httptest.ResponseRecorder
}{byApp: map[*adminApplication]*httptest.ResponseRecorder{}}

func publicationFixtureReviewer(t *testing.T, app *adminApplication) *httptest.ResponseRecorder {
	t.Helper()
	publicationFixtureReviewers.Lock()
	defer publicationFixtureReviewers.Unlock()
	if reviewer := publicationFixtureReviewers.byApp[app]; reviewer != nil {
		return reviewer
	}
	suffix := fmt.Sprint(publicationFixtureSequence.Add(1))
	reviewer := registerAccount(t, app, "publication.fixture."+suffix, "publication.fixture."+suffix+"@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "publication-fixture-role-"+suffix)
	publicationFixtureReviewers.byApp[app] = reviewer
	t.Cleanup(func() {
		publicationFixtureReviewers.Lock()
		delete(publicationFixtureReviewers.byApp, app)
		publicationFixtureReviewers.Unlock()
	})
	return reviewer
}

// Existing row-policy acceptance now observes a real create/submit/independent
// approval/execute path. This fixture returns the actual HTTP response unchanged.
// Failed approved fixture orders are checked then explicitly cancelled, so the
// next independent assertion can use that target. Retention itself is covered
// by TestPublicationAtomicPersistenceFailures without this cleanup. Successful
// fixtures explicitly complete before another independent mutation uses the record;
// callers still receive the unmodified historical execute response.
func publicationFixtureRequest(t *testing.T, app *adminApplication, operation, table, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	key := fmt.Sprintf("fixture-publication-%d", publicationFixtureSequence.Add(1))
	payload := map[string]json.RawMessage{}
	if body != "" {
		if err := json.Unmarshal([]byte(body), &payload); err != nil || payload == nil {
			return releaseRequest(t, app, "POST", "/api/v1/release-orders", body, key)
		}
	}
	title, exists := payload["title"]
	if exists {
		delete(payload, "title")
	} else {
		title, _ = json.Marshal(table + " fixture change")
	}
	item := map[string]any{"table_name": table, "operation": operation, "content": json.RawMessage(`{}`)}
	for name, value := range payload {
		if name == "expected_version" {
			item["expected_record_version"] = value
		} else {
			item[name] = value
		}
	}
	if operation != "ADD" {
		item["id"] = id
	}
	input, _ := json.Marshal(map[string]any{"title": title, "items": []any{item}})
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", string(input), key+"-create")
	if created.Code != 201 {
		return created
	}
	var order domain.ReleaseOrder
	if err := json.Unmarshal(created.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/release-orders/" + order.ID
	submitted := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, key+"-submit")
	if submitted.Code != 200 {
		return submitted
	}
	approved := releaseActorRequest(t, app, publicationFixtureReviewer(t, app), "POST", path+"/approve", `{"expected_version":"2","reason":"fixture acceptance review"}`, key+"-approve")
	if approved.Code != 200 {
		t.Fatalf("fixture approval: %d %s", approved.Code, approved.Body)
	}
	result := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, key+"-execute")
	if result.Code != 200 {
		current := releaseReadAllDetails(t, app, "GET", path, "", "")
		var stored domain.ReleaseOrder
		if current.Code != 200 || json.Unmarshal(current.Body.Bytes(), &stored) != nil || stored.State != "APPROVED" || stored.Version != "3" {
			t.Fatalf("failed publication did not retain approval: %d %s", current.Code, current.Body)
		}
		cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"3","reason":"end independent failure fixture"}`, key+"-cancel")
		if cancelled.Code != 200 {
			t.Fatalf("fixture cancellation: %d %s", cancelled.Code, cancelled.Body)
		}
	}
	if result.Code == 200 {
		completePublicationFixture(t, app, path, key+"-complete")
	}
	return result
}

// Ordinary policy tests explicitly inspect the current record before proposing
// a new change. Required-version and CAS tests use publicationFixtureRequest.
func versionedPublicationFixture(t *testing.T, app *adminApplication, operation, table, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]json.RawMessage{}
	if body == "" || json.Unmarshal([]byte(body), &payload) == nil && payload != nil {
		if _, exists := payload["expected_version"]; !exists {
			query, _ := json.Marshal(map[string]any{"conditions": []any{map[string]string{"field": "id", "operator": "exact", "value": id}}})
			read := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/"+table+"/query", string(query))
			var result struct {
				Versions []string `json:"record_versions"`
			}
			json.Unmarshal(read.Body.Bytes(), &result)
			version := "0"
			if len(result.Versions) == 1 {
				version = result.Versions[0]
			}
			payload["expected_version"], _ = json.Marshal(version)
			encoded, _ := json.Marshal(payload)
			body = string(encoded)
		}
	}
	return publicationFixtureRequest(t, app, operation, table, id, body)
}

func completePublicationFixture(t *testing.T, app *adminApplication, path, key string) domain.ReleaseOrder {
	t.Helper()
	return rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/complete", `{"expected_version":"4"}`, key), 200)
}
