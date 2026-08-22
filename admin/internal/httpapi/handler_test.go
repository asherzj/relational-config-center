package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/managedtable"
	"github.com/gin-gonic/gin"
)

type apiRepository struct {
	queryCalls  int
	createCalls int
	updateCalls int
	deleteCalls int
}

func (repository *apiRepository) Query(_ context.Context, _ managedtable.Policy, query managedtable.QuerySpec) (managedtable.PageResult, error) {
	repository.queryCalls++
	return managedtable.PageResult{
		Rows: []map[string]any{{"id": uint64(1), "name": "first"}},
		Page: managedtable.PageInfo{Number: query.Page.Number, Size: query.Page.Size, Total: 1},
	}, nil
}

func (repository *apiRepository) Create(_ context.Context, _ managedtable.Policy, _ map[string]any) (managedtable.MutationResult, error) {
	repository.createCalls++
	return managedtable.MutationResult{AffectedRows: 1, Key: "1"}, nil
}

func (repository *apiRepository) Update(_ context.Context, _ managedtable.Policy, key any, _ map[string]any) (managedtable.MutationResult, error) {
	repository.updateCalls++
	return managedtable.MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
}

func (repository *apiRepository) Delete(_ context.Context, _ managedtable.Policy, key any) (managedtable.MutationResult, error) {
	repository.deleteCalls++
	return managedtable.MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
}

type healthyPinger struct{}

func (healthyPinger) PingContext(context.Context) error { return nil }

func TestManagedTableHTTPFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &apiRepository{}
	router := testRouter(t, repository)

	metadata := performRequest(router, http.MethodGet, "/api/v1/tables/widgets", nil)
	if metadata.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, body = %s", metadata.Code, metadata.Body.String())
	}
	var metadataBody map[string]any
	if err := json.Unmarshal(metadata.Body.Bytes(), &metadataBody); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	encodedMetadata := metadata.Body.String()
	if bytes.Contains([]byte(encodedMetadata), []byte("app_widgets")) || bytes.Contains([]byte(encodedMetadata), []byte("widget_name")) {
		t.Fatalf("metadata exposes physical database names: %s", encodedMetadata)
	}

	query := performRequest(router, http.MethodPost, "/api/v1/tables/widgets/query", []byte(`{
		"filter":{"field":"name","operator":"contains","value":"fir"},
		"page":{"number":1,"size":10}
	}`))
	if query.Code != http.StatusOK {
		t.Fatalf("query status = %d, body = %s", query.Code, query.Body.String())
	}
	if repository.queryCalls != 1 {
		t.Fatalf("query calls = %d", repository.queryCalls)
	}

	create := performRequest(router, http.MethodPost, "/api/v1/tables/widgets/rows", []byte(`{
		"values":{"name":"first","data":{"enabled":true}}
	}`))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	if repository.createCalls != 1 {
		t.Fatalf("create calls = %d", repository.createCalls)
	}

	update := performRequest(router, http.MethodPatch, "/api/v1/tables/widgets/rows/1", []byte(`{
		"values":{"name":"renamed"}
	}`))
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", update.Code, update.Body.String())
	}
	if repository.updateCalls != 1 {
		t.Fatalf("update calls = %d", repository.updateCalls)
	}

	deleted := performRequest(router, http.MethodDelete, "/api/v1/tables/widgets/rows/1", nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	if repository.deleteCalls != 1 {
		t.Fatalf("delete calls = %d", repository.deleteCalls)
	}
}

func TestHTTPRejectsClientControlledIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &apiRepository{}
	router := testRouter(t, repository)
	response := performRequest(router, http.MethodPost, "/api/v1/tables/widgets/query", []byte(`{
		"filter":{"field":"name DESC; DROP TABLE app_widgets","operator":"eq","value":"x"},
		"page":{"number":1,"size":20}
	}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.queryCalls != 0 {
		t.Fatalf("query calls = %d, want 0", repository.queryCalls)
	}
}

func TestHTTPRejectsUnknownJSONFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := testRouter(t, &apiRepository{})
	response := performRequest(router, http.MethodPost, "/api/v1/tables/widgets/query", []byte(`{
		"sql":"SELECT * FROM app_widgets",
		"page":{"number":1,"size":20}
	}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func testRouter(t *testing.T, repository managedtable.Repository) http.Handler {
	t.Helper()
	registry, err := managedtable.NewRegistry(managedtable.Policy{
		Resource:    "widgets",
		Table:       "app_widgets",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		Fields: map[string]managedtable.FieldPolicy{
			"id": {
				Column:          "widget_id",
				Type:            managedtable.TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				AutoIncrement:   true,
				FilterOperators: []managedtable.Operator{managedtable.OperatorEqual},
			},
			"name": {
				Column:           "widget_name",
				Type:             managedtable.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
				Sortable:         true,
				FilterOperators:  []managedtable.Operator{managedtable.OperatorEqual, managedtable.OperatorContains},
			},
			"data": {
				Column:           "widget_data",
				Type:             managedtable.TypeJSON,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return NewRouter(managedtable.NewService(registry, repository), healthyPinger{})
}

func performRequest(handler http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
