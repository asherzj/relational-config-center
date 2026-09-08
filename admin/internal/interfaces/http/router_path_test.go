//go:build integration

package http_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

const textIDTable = "managed_text_ids"

// Business identities now travel in JSON. Encoded old row paths remain removed.
func TestRemovedMutationRoutesCannotBeRevivedByEncodedIdentity(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: io.Discard})
	for _, id := range []string{"键/值 ?#%", "%2F", ".", ".."} {
		for _, method := range []string{http.MethodPatch, http.MethodDelete} {
			response := performRequest(handler, method, "/api/v1/tables/"+textIDTable+"/rows/"+url.PathEscape(id), `{"content":{"value":"updated"}}`)
			assertSafeErrorEnvelope(t, response, http.StatusNotFound, "route_not_found")
		}
	}
}

func TestRawPathMutationRoutingPreservesAPIGuards(t *testing.T) {
	encodedID := url.PathEscape("键/值 ?#%")
	authenticatedHandler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: io.Discard})
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
	assertSafeErrorEnvelope(t, response, http.StatusNotFound, "route_not_found")
}
