//go:build integration

package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

const textIDTable = "managed_text_ids"

func TestManagedMutationRoutesDecodeRowIDExactlyOnce(t *testing.T) {
	executor := &recordingMutationExecutor{}
	handler := newTextIDHTTPHandler(t, httpinterface.RouterOptions{AccessLog: io.Discard}, executor)
	enableTextIDTable(t, handler)

	complexID := "键/值 ?#%"
	response := performRequest(handler, http.MethodPatch, "/api/v1/tables/"+textIDTable+"/rows/"+url.PathEscape(complexID), `{"content":{"value":"updated"}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("PATCH encoded complex id: HTTP %d %s", response.Code, response.Body.String())
	}
	if len(executor.updates) != 1 || executor.updates[0].ID != complexID {
		t.Fatalf("PATCH id = %#v, want exact decoded id %q", executor.updates, complexID)
	}

	response = performRequest(handler, http.MethodDelete, "/api/v1/tables/"+textIDTable+"/rows/"+url.PathEscape(complexID), "")
	if response.Code != http.StatusOK {
		t.Fatalf("DELETE encoded complex id: HTTP %d %s", response.Code, response.Body.String())
	}
	if len(executor.deletions) != 1 || executor.deletions[0].ID != complexID {
		t.Fatalf("DELETE id = %#v, want exact decoded id %q", executor.deletions, complexID)
	}

	literalEscape := "%2F"
	response = performRequest(handler, http.MethodPatch, "/api/v1/tables/"+textIDTable+"/rows/"+url.PathEscape(literalEscape), `{"content":{"value":"updated-again"}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("PATCH literal %%2F id: HTTP %d %s", response.Code, response.Body.String())
	}
	if len(executor.updates) != 2 || executor.updates[1].ID != literalEscape {
		t.Fatalf("PATCH literal escape id = %#v, want %q without double decoding", executor.updates, literalEscape)
	}
}

func TestRawPathMutationRoutingPreservesAPIGuards(t *testing.T) {
	encodedID := url.PathEscape("键/值 ?#%")
	authenticatedHandler := newTextIDHTTPHandler(t, httpinterface.RouterOptions{AccessLog: io.Discard}, &recordingMutationExecutor{})
	client := authenticatedHandler.(*authenticatedTestHandler)
	response := performRequest(client.Handler, http.MethodPatch, "/api/v1/tables/"+textIDTable+"/rows/"+encodedID, `{"content":{"value":"updated"}}`)
	assertSafeErrorEnvelope(t, response, http.StatusUnauthorized, "session_invalid")
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/tables/"+textIDTable+"/rows/"+encodedID, nil)
	for _, cookie := range client.cookies {
		if cookie.MaxAge >= 0 {
			request.AddCookie(cookie)
		}
	}
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	response = performHTTP(client.Handler, request)
	assertSafeErrorEnvelope(t, response, http.StatusForbidden, "csrf_invalid")

	response = performRequest(authenticatedHandler, http.MethodGet, "/api/v1/not-a-route/"+url.PathEscape("a/b"), "")
	assertSafeErrorEnvelope(t, response, http.StatusNotFound, "route_not_found")

	response = performRequest(authenticatedHandler, http.MethodDelete, "/api/v1/tables/rcc_catalog/rows/"+encodedID, "")
	assertSafeErrorEnvelope(t, response, http.StatusForbidden, "protected_table")
}

func newTextIDHTTPHandler(t *testing.T, options httpinterface.RouterOptions, executor *recordingMutationExecutor) http.Handler {
	t.Helper()
	adapter := newMemorySnapshotAdapter(memoryQueryExecutor{}, executor)
	adapter.tables[textIDTable] = domain.DescribeDatabaseTable(textIDTable, "Text identity configuration", []string{"id"}, false, false)
	adapter.schemaColumns[textIDTable] = []domain.Column{
		{Name: "id", Type: domain.ColumnTypeString},
		{Name: "value", Type: domain.ColumnTypeString},
	}
	return newPolicyHTTPHandlerWithSnapshotAdapter(t, options, adapter)
}

func enableTextIDTable(t *testing.T, handler http.Handler) {
	t.Helper()
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload(textIDTable)); response.Code != http.StatusCreated {
		t.Fatalf("create text-id Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/"+textIDTable+"/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable text-id Policy: HTTP %d %s", response.Code, response.Body.String())
	}
}

type recordingMutationExecutor struct {
	updates   []domain.RowUpdate
	deletions []domain.RowDelete
}

func (*recordingMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	return "unused", nil
}

func (executor *recordingMutationExecutor) UpdateRow(_ context.Context, update domain.RowUpdate) (int64, error) {
	executor.updates = append(executor.updates, update)
	return 1, nil
}

func (executor *recordingMutationExecutor) DeleteRow(_ context.Context, deletion domain.RowDelete) (int64, error) {
	executor.deletions = append(executor.deletions, deletion)
	return 1, nil
}
