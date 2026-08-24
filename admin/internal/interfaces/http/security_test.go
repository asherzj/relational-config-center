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

func TestAPIRequiresExactBearerTokenWhileHealthRemainsUnauthenticated(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{
		APIToken:  "deployment-secret",
		AccessLog: io.Discard,
	})

	if response := performRequest(handler, http.MethodGet, "/health/live", ""); response.Code != http.StatusOK {
		t.Fatalf("liveness must remain unauthenticated, got HTTP %d: %s", response.Code, response.Body.String())
	}

	for _, authorization := range []string{"", "Bearer wrong", "bearer deployment-secret", "Bearer deployment-secret extra"} {
		request := newRequest(http.MethodGet, "/api/v1/database-tables", "")
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		response := performHTTP(handler, request)
		assertSafeErrorEnvelope(t, response, http.StatusUnauthorized, "unauthorized")
	}

	request := newRequest(http.MethodGet, "/api/v1/database-tables", "")
	request.Header.Set("Authorization", "Bearer deployment-secret")
	if response := performHTTP(handler, request); response.Code != http.StatusOK {
		t.Fatalf("exact Bearer token should authorize request, got HTTP %d: %s", response.Code, response.Body.String())
	}
}

func TestAPIAccessLogIsStructuredAndRedactsRequestDetails(t *testing.T) {
	var logOutput bytes.Buffer
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{
		APIToken:  "deployment-secret",
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
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: &logOutput})
	response := performRequest(handler, http.MethodDelete, "/api/v1/tables/managed_alpha/rows/sensitive-row-id", "")
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
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: io.Discard})

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
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: io.Discard})
	body := `{"conditions":[],"padding":"` + strings.Repeat("x", (1<<20)+1) + `"}`
	response := performHTTP(handler, newRequest(http.MethodPost, "/api/v1/tables/managed_alpha/query", body))
	assertSafeErrorEnvelope(t, response, http.StatusBadRequest, "request_body_too_large")

	readOnlyRequest := newRequest(http.MethodGet, "/api/v1/database-tables", strings.Repeat("x", (1<<20)+1))
	readOnlyResponse := performHTTP(handler, readOnlyRequest)
	assertSafeErrorEnvelope(t, readOnlyResponse, http.StatusBadRequest, "request_body_too_large")
}

func TestAPICORSUsesExactOriginsAndPreflightDoesNotWeakenAuthentication(t *testing.T) {
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{
		APIToken:    "deployment-secret",
		CORSOrigins: []string{"https://admin.example.test"},
		AccessLog:   io.Discard,
	})

	preflight := newRequest(http.MethodOptions, "/api/v1/database-tables", "")
	preflight.Header.Set("Origin", "https://admin.example.test")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflight.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type, X-Request-ID")
	preflightResponse := performHTTP(handler, preflight)
	if preflightResponse.Code != http.StatusNoContent || preflightResponse.Header().Get("Access-Control-Allow-Origin") != "https://admin.example.test" {
		t.Fatalf("expected valid unauthenticated preflight, got HTTP %d headers %#v body %s", preflightResponse.Code, preflightResponse.Header(), preflightResponse.Body.String())
	}

	actual := newRequest(http.MethodGet, "/api/v1/database-tables", "")
	actual.Header.Set("Origin", "https://admin.example.test")
	actualResponse := performHTTP(handler, actual)
	assertSafeErrorEnvelope(t, actualResponse, http.StatusUnauthorized, "unauthorized")
	if actualResponse.Header().Get("Access-Control-Allow-Origin") != "https://admin.example.test" {
		t.Fatalf("allowed actual origin must receive CORS response header: %#v", actualResponse.Header())
	}

	for _, rejectedOrigin := range []string{"https://admin.example.test.evil", "https://ADMIN.example.test", "*"} {
		request := newRequest(http.MethodGet, "/api/v1/database-tables", "")
		request.Header.Set("Origin", rejectedOrigin)
		request.Header.Set("Authorization", "Bearer deployment-secret")
		response := performHTTP(handler, request)
		assertSafeErrorEnvelope(t, response, http.StatusForbidden, "cors_origin_forbidden")
		if response.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("rejected origin must not be reflected: %#v", response.Header())
		}
	}
}

func TestAPIPanicRecoveryReturnsStableSafeInternalError(t *testing.T) {
	handler := newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: io.Discard}, panicMutationExecutor{})
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
			handler := newPolicyHTTPHandlerWithExecutors(t, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: io.Discard}, errorQueryExecutor{err: test.err}, memoryMutationExecutor{})
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
	handler := newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: io.Discard})
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
			handler := newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: io.Discard}, errorMutationExecutor{err: test.err})
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
