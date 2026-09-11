//go:build integration

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	passwordadapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
	driver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

func newPolicyHTTPHandlerWithRouterOptions(t *testing.T, options httpinterface.RouterOptions) http.Handler {
	return newPolicyHTTPHandlerWithExecutors(t, options, memoryQueryExecutor{})
}

func newPolicyHTTPHandlerWithExecutors(t *testing.T, options httpinterface.RouterOptions, queryExecutor application.QueryExecutor, initialPolicies ...domain.TablePolicy) http.Handler {
	t.Helper()
	adapter := newMemorySnapshotAdapter(queryExecutor)
	for _, policy := range initialPolicies {
		adapter.tablePolicies[policy.TableName] = policy
	}
	queryPolicies := application.NewQueryPolicyManagement(adapter, application.NewQueryPolicyTypeRegistry())
	mutationPolicies := application.NewMutationPolicyManagement(adapter, application.NewMutationPolicyTypeRegistry())
	policies := application.NewTablePolicyManagement(adapter, adapter, queryPolicies, mutationPolicies)
	queries := application.NewManagedTableQuery(adapter, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry())
	authStore := securityAccountStore(t)
	options.Authentication = application.NewAuthentication(authStore, passwordadapter.NewArgon2id(), nil, authStore, application.AuthenticationLimits{})
	options.AccountHTTP = httpinterface.AccountHTTPOptions{PublicOrigin: "http://127.0.0.1:5173", InsecureLocalHTTP: true}
	handler := httpinterface.NewRouter(application.NewDatabaseTableDiscovery(adapter), readyAdapter{}, queryPolicies, mutationPolicies, policies, queries, options)
	client := registerSecurityAdminClient(t, handler, authStore)
	if log, ok := options.AccessLog.(*bytes.Buffer); ok {
		log.Reset()
	}
	return client
}

func validPolicyPayload(tableName string) string {
	return `{"table_name":"` + tableName + `","query_policy_code":"test_page_query_v1","mutation_policy_code":"test_mutation_v1"}`
}

func performRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
	}
	return performHTTP(handler, request)
}

type memorySnapshotAdapter struct {
	mu               sync.Mutex
	tables           map[string]domain.DatabaseTable
	tablePolicies    map[string]domain.TablePolicy
	queryPolicies    map[string]domain.QueryPolicy
	mutationPolicies map[string]domain.MutationPolicy
	queryExecutor    application.QueryExecutor
}

func newMemorySnapshotAdapter(queryExecutor application.QueryExecutor) *memorySnapshotAdapter {
	return &memorySnapshotAdapter{
		tables: map[string]domain.DatabaseTable{
			"managed_alpha": domain.DescribeDatabaseTable("managed_alpha", "Alpha configuration", []string{"id"}, false, false),
			"incompatible":  domain.DescribeDatabaseTable("incompatible", "Incompatible", []string{"code"}, false, false),
		},
		tablePolicies: make(map[string]domain.TablePolicy),
		queryPolicies: map[string]domain.QueryPolicy{
			"test_page_query_v1": {Code: "test_page_query_v1", Name: "Test query", TypeCode: application.PageQueryPolicyType, DefaultOrderField: "id", DefaultOrderDirection: "DESC", DefaultPageSize: 20, MaxPageSize: 200, Status: domain.PolicyStatusActive},
		},
		mutationPolicies: map[string]domain.MutationPolicy{
			"test_mutation_v1": {Code: "test_mutation_v1", Name: "Test mutation", TypeCode: application.SingleTableMutationPolicyType, AllowAdd: true, AllowModify: true, AllowDelete: true, Status: domain.PolicyStatusActive},
		},
		queryExecutor: queryExecutor,
	}
}

func (adapter *memorySnapshotAdapter) ListDatabaseTables(context.Context) ([]domain.DatabaseTable, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	tables := make([]domain.DatabaseTable, 0, len(adapter.tables))
	for _, table := range adapter.tables {
		policy, found := adapter.tablePolicies[table.Name]
		table.PolicyExists, table.PolicyEnabled = found, found && policy.Enabled
		tables = append(tables, table)
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Name < tables[j].Name })
	return tables, nil
}

func (adapter *memorySnapshotAdapter) GetDatabaseTable(_ context.Context, name string) (domain.DatabaseTable, error) {
	if strings.HasPrefix(strings.ToLower(name), "rcc_") {
		return domain.DatabaseTable{}, application.ErrProtectedTable
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	table, found := adapter.tables[name]
	if !found {
		return domain.DatabaseTable{}, application.ErrDatabaseTableNotFound
	}
	return table, nil
}

func (adapter *memorySnapshotAdapter) GetTableSchema(ctx context.Context, name string) (domain.TableSchema, error) {
	table, err := adapter.GetDatabaseTable(ctx, name)
	if err != nil {
		return domain.TableSchema{}, err
	}
	return domain.TableSchema{Name: name, Compatible: table.Compatible, IncompatibilityReason: table.IncompatibilityReason, Columns: []domain.Column{
		{Name: "id", Type: domain.ColumnTypeUInt64, AutoIncrement: true},
		{Name: "value", Type: domain.ColumnTypeString},
	}}, nil
}

func (adapter *memorySnapshotAdapter) Create(_ context.Context, policy domain.TablePolicy, operator string) error {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if _, found := adapter.tablePolicies[policy.TableName]; found {
		return domain.ErrTablePolicyExists
	}
	policy.Creator, policy.Modifier = operator, operator
	adapter.tablePolicies[policy.TableName] = policy
	return nil
}

func (adapter *memorySnapshotAdapter) List(context.Context) ([]domain.TablePolicy, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	result := make([]domain.TablePolicy, 0, len(adapter.tablePolicies))
	for _, policy := range adapter.tablePolicies {
		result = append(result, policy)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TableName < result[j].TableName })
	return result, nil
}

func (adapter *memorySnapshotAdapter) Get(_ context.Context, name string) (domain.TablePolicy, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	policy, found := adapter.tablePolicies[name]
	if !found {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	return policy, nil
}

func (adapter *memorySnapshotAdapter) Replace(_ context.Context, replacement domain.TablePolicy, operator string) (domain.TablePolicy, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	current, found := adapter.tablePolicies[replacement.TableName]
	if !found {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	replacement.Enabled, replacement.Creator, replacement.Modifier = current.Enabled, current.Creator, operator
	adapter.tablePolicies[replacement.TableName] = replacement
	return replacement, nil
}

func (adapter *memorySnapshotAdapter) SetEnabled(_ context.Context, name string, enabled bool, operator string) (domain.TablePolicy, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	policy, found := adapter.tablePolicies[name]
	if !found {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	policy.Enabled, policy.Modifier = enabled, operator
	adapter.tablePolicies[name] = policy
	return policy, nil
}

func (adapter *memorySnapshotAdapter) CreateQueryPolicy(_ context.Context, policy domain.QueryPolicy, operator string) (domain.QueryPolicy, error) {
	adapter.queryPolicies[policy.Code] = policy
	return policy, nil
}
func (adapter *memorySnapshotAdapter) ListQueryPolicies(context.Context) ([]domain.QueryPolicy, error) {
	return nil, nil
}
func (adapter *memorySnapshotAdapter) GetQueryPolicy(_ context.Context, code string) (domain.QueryPolicy, error) {
	policy, ok := adapter.queryPolicies[code]
	if !ok {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyNotFound
	}
	return policy, nil
}
func (adapter *memorySnapshotAdapter) ReplaceDraftQueryPolicy(_ context.Context, policy domain.QueryPolicy, _ string) (domain.QueryPolicy, error) {
	adapter.queryPolicies[policy.Code] = policy
	return policy, nil
}
func (adapter *memorySnapshotAdapter) SetQueryPolicyStatus(_ context.Context, code string, _, to domain.PolicyStatus, _ string) (domain.QueryPolicy, error) {
	policy, err := adapter.GetQueryPolicy(context.Background(), code)
	policy.Status = to
	adapter.queryPolicies[code] = policy
	return policy, err
}
func (adapter *memorySnapshotAdapter) UpdateQueryPolicyMetadata(_ context.Context, code, name, description, _ string) (domain.QueryPolicy, error) {
	policy, err := adapter.GetQueryPolicy(context.Background(), code)
	policy.Name, policy.Description = name, description
	adapter.queryPolicies[code] = policy
	return policy, err
}
func (adapter *memorySnapshotAdapter) DeleteDraftQueryPolicy(_ context.Context, code string) error {
	delete(adapter.queryPolicies, code)
	return nil
}

func (adapter *memorySnapshotAdapter) CreateMutationPolicy(_ context.Context, policy domain.MutationPolicy, _ string) (domain.MutationPolicy, error) {
	adapter.mutationPolicies[policy.Code] = policy
	return policy, nil
}
func (adapter *memorySnapshotAdapter) ListMutationPolicies(context.Context) ([]domain.MutationPolicy, error) {
	return nil, nil
}
func (adapter *memorySnapshotAdapter) GetMutationPolicy(_ context.Context, code string) (domain.MutationPolicy, error) {
	policy, ok := adapter.mutationPolicies[code]
	if !ok {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyNotFound
	}
	return policy, nil
}
func (adapter *memorySnapshotAdapter) ReplaceDraftMutationPolicy(_ context.Context, policy domain.MutationPolicy, _ string) (domain.MutationPolicy, error) {
	adapter.mutationPolicies[policy.Code] = policy
	return policy, nil
}
func (adapter *memorySnapshotAdapter) SetMutationPolicyStatus(_ context.Context, code string, _, to domain.PolicyStatus, _ string) (domain.MutationPolicy, error) {
	policy, err := adapter.GetMutationPolicy(context.Background(), code)
	policy.Status = to
	adapter.mutationPolicies[code] = policy
	return policy, err
}
func (adapter *memorySnapshotAdapter) UpdateMutationPolicyMetadata(_ context.Context, code, name, description, _ string) (domain.MutationPolicy, error) {
	policy, err := adapter.GetMutationPolicy(context.Background(), code)
	policy.Name, policy.Description = name, description
	adapter.mutationPolicies[code] = policy
	return policy, err
}
func (adapter *memorySnapshotAdapter) DeleteDraftMutationPolicy(_ context.Context, code string) error {
	delete(adapter.mutationPolicies, code)
	return nil
}

func (adapter *memorySnapshotAdapter) ExecuteQuerySnapshot(ctx context.Context, execute func(application.QuerySnapshotSession) (domain.QueryResult, error)) (domain.QueryResult, error) {
	return execute((*memorySnapshotSession)(adapter))
}

type memorySnapshotSession memorySnapshotAdapter

func (session *memorySnapshotSession) GetTablePolicy(ctx context.Context, name string) (domain.TablePolicy, error) {
	return (*memorySnapshotAdapter)(session).Get(ctx, name)
}
func (session *memorySnapshotSession) GetQueryPolicy(ctx context.Context, code string) (domain.QueryPolicy, error) {
	return (*memorySnapshotAdapter)(session).GetQueryPolicy(ctx, code)
}
func (session *memorySnapshotSession) GetMutationPolicy(ctx context.Context, code string) (domain.MutationPolicy, error) {
	return (*memorySnapshotAdapter)(session).GetMutationPolicy(ctx, code)
}
func (session *memorySnapshotSession) GetTableSchema(ctx context.Context, name string) (domain.TableSchema, error) {
	return (*memorySnapshotAdapter)(session).GetTableSchema(ctx, name)
}
func (session *memorySnapshotSession) ExecutePageQuery(ctx context.Context, query domain.PageQuery) (domain.QueryResult, error) {
	return session.queryExecutor.ExecutePageQuery(ctx, query)
}

type readyAdapter struct{}

func (readyAdapter) Ready(context.Context) error { return nil }

type memoryQueryExecutor struct{}

func (memoryQueryExecutor) ExecutePageQuery(context.Context, domain.PageQuery) (domain.QueryResult, error) {
	return domain.QueryResult{}, nil
}

var _ application.QuerySnapshotExecutor = (*memorySnapshotAdapter)(nil)
var _ domain.TablePolicyCatalog = (*memorySnapshotAdapter)(nil)
var _ domain.QueryPolicyCatalog = (*memorySnapshotAdapter)(nil)
var _ domain.MutationPolicyCatalog = (*memorySnapshotAdapter)(nil)

// This test client obtains and sends actual public Cookie/CSRF credentials.
// The embedded server handler itself never injects or bypasses authentication.
type authenticatedTestHandler struct {
	http.Handler
	cookies []*http.Cookie
	csrf    string
}

func registerSecurityAdminClient(t *testing.T, handler http.Handler, store *mysqladapter.Adapter) *authenticatedTestHandler {
	t.Helper()
	send := func(method, path, body string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://127.0.0.1:5173")
		request.Header.Set("X-CSRF-Token", csrf)
		for _, cookie := range cookies {
			if cookie.MaxAge >= 0 {
				request.AddCookie(cookie)
			}
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	prepare := send("GET", "/api/v1/auth/csrf", "", nil, "")
	var challenge struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(prepare.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	registered := send("POST", "/api/v1/auth/register", `{"username":"security.user","email":"security@example.com","password":"correct horse battery staple"}`, prepare.Result().Cookies(), challenge.CSRF)
	if registered.Code != 201 {
		t.Fatalf("register real security test account: %d %s", registered.Code, registered.Body.String())
	}
	// Writer regression fixtures explicitly grant ADMIN, never change registration defaults.
	if _, err := application.NewAccountMaintenance(store, passwordadapter.NewArgon2id()).GrantAdmin(t.Context(), application.AccountSelector{Username: "security.user"}); err != nil {
		t.Fatal(err)
	}
	// Normal test clients authenticate with the public login flow as scripts do.
	prepare = send("GET", "/api/v1/auth/csrf", "", nil, "")
	if err := json.Unmarshal(prepare.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	login := send("POST", "/api/v1/auth/login", `{"username":"security.user","password":"correct horse battery staple"}`, prepare.Result().Cookies(), challenge.CSRF)
	if login.Code != 200 {
		t.Fatalf("login security test account: %d %s", login.Code, login.Body.String())
	}
	if err := json.Unmarshal(login.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	return &authenticatedTestHandler{Handler: handler, cookies: login.Result().Cookies(), csrf: challenge.CSRF}
}

func securityAccountStore(t *testing.T) *mysqladapter.Adapter {
	t.Helper()
	ctx := t.Context()
	container, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithDatabase("rcc_test"), tcmysql.WithUsername("rcc_admin"), tcmysql.WithPassword("rcc_password"))
	if err != nil {
		t.Fatalf("real authentication MySQL: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})
	parsed, err := driver.ParseDSN(container.MustConnectionString(ctx, "parseTime=true"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := mysqladapter.OpenMaintenance(ctx, config.MySQL{Network: parsed.Net, Address: parsed.Addr, Database: parsed.DBName, User: parsed.User, Password: parsed.Passwd, TLSMode: "false", MaxOpenConnections: 4, MaxIdleConnections: 4, ConnectTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(fmt.Errorf("open auth store: %w", err))
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.MigrateControlSchema(ctx, mysqladapter.SchemaMigrationOptions{LockTimeout: 5 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if err := store.Ready(ctx); err != nil {
		t.Fatal(err)
	}

	return store
}
