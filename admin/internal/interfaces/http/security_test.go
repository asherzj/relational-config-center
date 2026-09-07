//go:build integration

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

func TestAPIRejectsOldBearerCredentialsWhileHealthRemainsPublic(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: io.Discard})
	for _, token := range []string{"", "Bearer deployment-secret"} {
		request := newRequest("GET", "/api/v1/database-tables", "")
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assertSafeErrorEnvelope(t, response, 401, "session_invalid")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newRequest("GET", "/health/live", ""))
	if response.Code != 200 {
		t.Fatalf("public health: %d", response.Code)
	}
	if response := performRequest(handler, "GET", "/api/v1/database-tables", ""); response.Code != 200 {
		t.Fatalf("real session: %d %s", response.Code, response.Body.String())
	}
}

func TestAPIAccessLogIsStructuredAndRedactsRequestDetails(t *testing.T) {
	var logOutput bytes.Buffer
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{
		AccessLog: &logOutput,
	})
	request := newRequest(http.MethodPost, "/api/v1/tables/managed_alpha/query?query_secret=do-not-log", `{"conditions":[{"field":"value","operator":"exact","value":"row-secret"}]}`)
	request.Header.Set("Authorization", "Bearer deployment-secret")
	request.Header.Set("X-Request-ID", "log-test-1")
	response := performHTTP(handler, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected Policy lookup result, got HTTP %d: %s", response.Code, response.Body.String())
	}

	logLine := strings.TrimSpace(logOutput.String())
	for _, forbidden := range []string{"deployment-secret", "row-secret", "query_secret", "do-not-log", "Authorization", "SELECT", "DSN"} {
		if strings.Contains(logLine, forbidden) {
			t.Fatalf("access log leaked %q: %s", forbidden, logLine)
		}
	}
	var event struct {
		RequestID  string `json:"request_id"`
		Method     string `json:"method"`
		Path       string `json:"path"`
		Status     int    `json:"status"`
		DurationMS int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal([]byte(logLine), &event); err != nil {
		t.Fatalf("access log is not one JSON object: %v; %q", err, logLine)
	}
	if event.RequestID != "log-test-1" || event.Method != http.MethodPost || event.Path != "/api/v1/tables/:table_name/query" || event.Status != http.StatusNotFound || event.DurationMS < 0 {
		t.Fatalf("unexpected access log event: %#v", event)
	}
}

func TestAPIAccessLogUsesRouteTemplatesWithoutDynamicTableOrRowValues(t *testing.T) {
	var logOutput bytes.Buffer
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: &logOutput})
	response := performRequest(handler, http.MethodDelete, "/api/v1/tables/managed_alpha/rows/sensitive-row-id", `{"expected_version":"0"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected Policy lookup result, got HTTP %d: %s", response.Code, response.Body.String())
	}
	logLine := strings.TrimSpace(logOutput.String())
	if strings.Contains(logLine, "managed_alpha") || strings.Contains(logLine, "sensitive-row-id") {
		t.Fatalf("access log leaked dynamic route values: %s", logLine)
	}
	var event struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(logLine), &event); err != nil {
		t.Fatalf("decode access log: %v; %q", err, logLine)
	}
	if event.Path != "/api/v1/tables/:table_name/rows/:id" {
		t.Fatalf("expected stable route template, got %q", event.Path)
	}

	logOutput.Reset()
	unknown := performRequest(handler, http.MethodGet, "/api/v1/unknown/secret-segment", "")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("expected unknown route rejection, got HTTP %d", unknown.Code)
	}
	if logLine = strings.TrimSpace(logOutput.String()); strings.Contains(logLine, "secret-segment") || !strings.Contains(logLine, `"path":"unmatched"`) {
		t.Fatalf("unknown route log did not use safe placeholder: %s", logLine)
	}
}

func TestAPIRequestIDAcceptsOnlyTightlyValidatedValues(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: io.Discard})

	valid := newRequest(http.MethodGet, "/api/v1/database-tables", "")
	valid.Header.Set("X-Request-ID", "client_ABC-123.9")
	validResponse := performHTTP(handler, valid)
	if got := validResponse.Header().Get("X-Request-ID"); got != "client_ABC-123.9" {
		t.Fatalf("expected validated incoming request ID to be retained, got %q", got)
	}

	requestIDPattern := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	for _, invalid := range []string{"../../secret", "contains space", strings.Repeat("x", 65)} {
		request := newRequest(http.MethodGet, "/api/v1/database-tables", "")
		request.Header.Set("X-Request-ID", invalid)
		response := performHTTP(handler, request)
		generated := response.Header().Get("X-Request-ID")
		if generated == invalid || !requestIDPattern.MatchString(generated) {
			t.Fatalf("expected a safe generated request ID for %q, got %q", invalid, generated)
		}
	}
}

func TestAPIBodyLimitRejectsOversizedJSONWithStableError(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: io.Discard})
	body := `{"conditions":[],"padding":"` + strings.Repeat("x", (1<<20)+1) + `"}`
	response := performHTTP(handler, newRequest(http.MethodPost, "/api/v1/tables/managed_alpha/query", body))
	assertSafeErrorEnvelope(t, response, http.StatusBadRequest, "request_body_too_large")

	readOnlyRequest := newRequest(http.MethodGet, "/api/v1/database-tables", strings.Repeat("x", (1<<20)+1))
	readOnlyResponse := performHTTP(handler, readOnlyRequest)
	assertSafeErrorEnvelope(t, readOnlyResponse, http.StatusBadRequest, "request_body_too_large")
}

func TestAPIDoesNotOfferCrossOriginCredentialAccess(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: io.Discard})
	request := newRequest("OPTIONS", "/api/v1/database-tables", "")
	request.Header.Set("Origin", "https://external.example")
	request.Header.Set("Access-Control-Request-Method", "GET")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertSafeErrorEnvelope(t, response, 401, "session_invalid")
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("cross-origin access advertised")
	}
}

func TestAPIPanicRecoveryReturnsStableSafeInternalError(t *testing.T) {
	handler := newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t, httpinterface.RouterOptions{AccessLog: io.Discard}, panicMutationExecutor{})
	payload := strings.Replace(validPolicyPayload("managed_alpha"), `"allow_add":false`, `"allow_add":true`, 1)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	response := performHTTP(handler, newRequest(http.MethodPost, "/api/v1/tables/managed_alpha/rows", `{"content":{"value":"secret-row-value"}}`))
	assertSafeErrorEnvelope(t, response, http.StatusInternalServerError, "internal_error")
	if strings.Contains(response.Body.String(), "mutation executor") || strings.Contains(response.Body.String(), "secret-row-value") {
		t.Fatalf("panic response leaked details: %s", response.Body.String())
	}
}

func TestAPIMapsTimeoutUnavailableAndUnclassifiedErrorsSafely(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "deadline", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: "query_timeout"},
		{name: "known unavailable", err: application.ErrQueryUnavailable, status: http.StatusServiceUnavailable, code: "query_unavailable"},
		{name: "unclassified internal", err: errors.New("driver detail secret"), status: http.StatusInternalServerError, code: "internal_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newPolicyHTTPHandlerWithExecutors(t, httpinterface.RouterOptions{AccessLog: io.Discard}, errorQueryExecutor{err: test.err}, memoryMutationExecutor{})
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha")); response.Code != http.StatusCreated {
				t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
			}
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
				t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
			}

			response := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/query", `{}`)
			assertSafeErrorEnvelope(t, response, test.status, test.code)
			if strings.Contains(response.Body.String(), "driver detail") {
				t.Fatalf("error response leaked infrastructure details: %s", response.Body.String())
			}
		})
	}
}

func TestAPIUnknownRoutesUseTheSafeErrorEnvelope(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AccessLog: io.Discard})
	response := performRequest(handler, http.MethodGet, "/api/v1/does-not-exist", "")
	assertSafeErrorEnvelope(t, response, http.StatusNotFound, "route_not_found")
}

func TestAPIMutationTimeoutAndInternalErrorsUseSafeMappings(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "deadline", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: "mutation_timeout"},
		{name: "known unavailable", err: application.ErrMutationUnavailable, status: http.StatusServiceUnavailable, code: "mutation_unavailable"},
		{name: "unclassified internal", err: errors.New("bound row value secret"), status: http.StatusInternalServerError, code: "internal_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t, httpinterface.RouterOptions{AccessLog: io.Discard}, errorMutationExecutor{err: test.err})
			payload := strings.Replace(validPolicyPayload("managed_alpha"), `"allow_add":false`, `"allow_add":true`, 1)
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
				t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
			}
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
				t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
			}
			response := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/rows", `{"content":{"value":"client-secret"}}`)
			assertSafeErrorEnvelope(t, response, test.status, test.code)
			if strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("mutation error leaked details: %s", response.Body.String())
			}
		})
	}
}

type errorQueryExecutor struct {
	err error
}

func (executor errorQueryExecutor) ExecutePageQuery(context.Context, domain.PageQuery) (domain.QueryResult, error) {
	return domain.QueryResult{}, executor.err
}

var _ application.QueryExecutor = errorQueryExecutor{}

type errorMutationExecutor struct {
	err error
}

func (executor errorMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	return "", executor.err
}
func (executor errorMutationExecutor) UpdateRow(context.Context, domain.RowUpdate) (int64, error) {
	return 0, executor.err
}
func (executor errorMutationExecutor) DeleteRow(context.Context, domain.RowDelete) (int64, error) {
	return 0, executor.err
}

var _ application.MutationExecutor = errorMutationExecutor{}

func newRequest(method, path, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, path, nil)
	}
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func performHTTP(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	if client, ok := handler.(*authenticatedTestHandler); ok {
		for _, cookie := range client.cookies {
			if cookie.MaxAge >= 0 {
				request.AddCookie(cookie)
			}
		}
		request.Header.Set("Origin", "http://127.0.0.1:5173")
		request.Header.Set("X-CSRF-Token", client.csrf)
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertSafeErrorEnvelope(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected HTTP %d, got %d: %s", status, response.Code, response.Body.String())
	}
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != code || envelope.Error.Message == "" || envelope.Error.RequestID == "" {
		t.Fatalf("unexpected safe error envelope: %#v", envelope)
	}
	if response.Header().Get("X-Request-ID") != envelope.Error.RequestID {
		t.Fatalf("request ID header/body mismatch: header=%q body=%q", response.Header().Get("X-Request-ID"), envelope.Error.RequestID)
	}
}
