package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"github.com/gin-gonic/gin"
)

type apiRepository struct {
	queryCalls  int
	createCalls int
	updateCalls int
	deleteCalls int
}

func (repository *apiRepository) Query(_ context.Context, _ domain.Policy, query domain.QuerySpec) (domain.PageResult, error) {
	repository.queryCalls++
	return domain.PageResult{
		Rows: []map[string]any{{"id": uint64(1), "name": "first"}},
		Page: domain.PageInfo{Number: query.Page.Number, Size: query.Page.Size, Total: 1},
	}, nil
}

func (repository *apiRepository) Create(_ context.Context, _ domain.Policy, _ map[string]any) (domain.MutationResult, error) {
	repository.createCalls++
	return domain.MutationResult{AffectedRows: 1, Key: "1"}, nil
}

func (repository *apiRepository) Update(_ context.Context, _ domain.Policy, key any, _ map[string]any) (domain.MutationResult, error) {
	repository.updateCalls++
	return domain.MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
}

func (repository *apiRepository) Delete(_ context.Context, _ domain.Policy, key any) (domain.MutationResult, error) {
	repository.deleteCalls++
	return domain.MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
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

func testRouter(t *testing.T, repository domain.Repository) http.Handler {
	t.Helper()
	registry, err := domain.NewRegistry(widgetsPolicy())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	source := application.NewPolicySource(registry)
	return NewRouter(
		application.NewService(source, repository),
		application.NewCatalogService(newFakePolicyRepository(), okayVerifier{}, source),
		healthyPinger{},
	)
}

type fakePolicyRepository struct {
	policies map[string]domain.Policy
}

func newFakePolicyRepository(policies ...domain.Policy) *fakePolicyRepository {
	repository := &fakePolicyRepository{policies: make(map[string]domain.Policy, len(policies))}
	for _, policy := range policies {
		repository.policies[policy.Resource] = policy
	}
	return repository
}

func (repository *fakePolicyRepository) Load(context.Context) ([]domain.Policy, error) {
	names := make([]string, 0, len(repository.policies))
	for name := range repository.policies {
		names = append(names, name)
	}
	sort.Strings(names)
	policies := make([]domain.Policy, 0, len(names))
	for _, name := range names {
		policies = append(policies, repository.policies[name])
	}
	return policies, nil
}

func (repository *fakePolicyRepository) Save(_ context.Context, policy domain.Policy) error {
	repository.policies[policy.Resource] = policy
	return nil
}

func (repository *fakePolicyRepository) Delete(_ context.Context, resource string) error {
	if _, exists := repository.policies[resource]; !exists {
		return domain.ErrPolicyNotFound
	}
	delete(repository.policies, resource)
	return nil
}

type okayVerifier struct{}

func (okayVerifier) VerifyPolicy(context.Context, domain.Policy) error { return nil }

func performRequest(handler http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestPolicyCatalogEndpointsManageRuntimePolicies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry, err := domain.NewRegistry(widgetsPolicy())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	source := application.NewPolicySource(registry)
	policyRepository := newFakePolicyRepository()
	router := NewRouter(
		application.NewService(source, &apiRepository{}),
		application.NewCatalogService(policyRepository, okayVerifier{}, source),
		healthyPinger{},
	)

	// The unregistered gadgets table is invisible before a policy exists.
	if response := performRequest(router, http.MethodGet, "/api/v1/tables/gadgets", nil); response.Code != http.StatusNotFound {
		t.Fatalf("gadgets before policy: status = %d, body = %s", response.Code, response.Body.String())
	}

	save := performRequest(router, http.MethodPut, "/api/v1/policies/gadgets", []byte(`{
		"resource": "gadgets",
		"table": "gadgets_table",
		"primary_key": "id",
		"fields": {
			"id": {"column": "gadget_id", "type": "unsigned_integer", "readable": true, "sortable": true, "auto_increment": true},
			"name": {"column": "gadget_name", "type": "string", "readable": true, "creatable": true, "updatable": true, "sortable": true, "required_on_create": true, "min_length": 1, "max_length": 64}
		},
		"allow_create": true,
		"allow_update": true,
		"allow_delete": true
	}`))
	if save.Code != http.StatusOK {
		t.Fatalf("save policy status = %d, body = %s", save.Code, save.Body.String())
	}

	// Immediately active: discovery and the generic table API see gadgets
	// without any restart or rebuild.
	if response := performRequest(router, http.MethodGet, "/api/v1/tables", nil); response.Code != http.StatusOK ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"gadgets"`)) {
		t.Fatalf("tables after save = %s", response.Body.String())
	}
	if response := performRequest(router, http.MethodGet, "/api/v1/tables/gadgets", nil); response.Code != http.StatusOK {
		t.Fatalf("gadgets after save: status = %d, body = %s", response.Code, response.Body.String())
	}

	// The saved document round-trips through the catalog API.
	fetched := performRequest(router, http.MethodGet, "/api/v1/policies/gadgets", nil)
	if fetched.Code != http.StatusOK || !bytes.Contains(fetched.Body.Bytes(), []byte(`"gadget_name"`)) {
		t.Fatalf("get policy = %d, %s", fetched.Code, fetched.Body.String())
	}

	// Resource in the body must match the URL.
	mismatch := performRequest(router, http.MethodPut, "/api/v1/policies/other", []byte(`{"resource": "gadgets", "table": "gadgets_table", "primary_key": "id", "fields": {}}`))
	if mismatch.Code != http.StatusBadRequest {
		t.Fatalf("resource mismatch status = %d, body = %s", mismatch.Code, mismatch.Body.String())
	}

	// The catalog can never manage itself through any API.
	selfManage := performRequest(router, http.MethodPut, "/api/v1/policies/catalog", []byte(`{"resource": "catalog", "table": "table_policies", "primary_key": "resource", "fields": {"resource": {"column": "resource", "type": "string", "readable": true, "creatable": true, "required_on_create": true}}}`))
	if selfManage.Code != http.StatusBadRequest {
		t.Fatalf("self-management status = %d, body = %s", selfManage.Code, selfManage.Body.String())
	}

	// The generic table API cannot reach the catalog either.
	if response := performRequest(router, http.MethodGet, "/api/v1/tables/table_policies", nil); response.Code != http.StatusNotFound {
		t.Fatalf("generic API reaches catalog: status = %d", response.Code)
	}

	// Unknown JSON fields (including anything datasource-shaped) are rejected.
	unknown := performRequest(router, http.MethodPut, "/api/v1/policies/gadgets", []byte(`{"resource": "gadgets", "dsn": "mysql://evil"}`))
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, body = %s", unknown.Code, unknown.Body.String())
	}

	// Deleting the policy deactivates the resource on the next request.
	deleted := performRequest(router, http.MethodDelete, "/api/v1/policies/gadgets", nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete policy status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	if response := performRequest(router, http.MethodGet, "/api/v1/tables/gadgets", nil); response.Code != http.StatusNotFound {
		t.Fatalf("gadgets after delete: status = %d", response.Code)
	}
	if response := performRequest(router, http.MethodGet, "/api/v1/policies/gadgets", nil); response.Code != http.StatusNotFound {
		t.Fatalf("policy after delete: status = %d", response.Code)
	}
}

func widgetsPolicy() domain.Policy {
	return domain.Policy{
		Resource:    "widgets",
		Table:       "app_widgets",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		Fields: map[string]domain.FieldPolicy{
			"id": {
				Column:          "widget_id",
				Type:            domain.TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				AutoIncrement:   true,
				FilterOperators: []domain.Operator{domain.OperatorEqual},
			},
			"name": {
				Column:           "widget_name",
				Type:             domain.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
				Sortable:         true,
				FilterOperators:  []domain.Operator{domain.OperatorEqual, domain.OperatorContains},
			},
			"data": {
				Column:           "widget_data",
				Type:             domain.TypeJSON,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
			},
		},
	}
}
