package http_test

import (
	"bytes"
	"context"
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
	return newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t, options, memoryMutationExecutor{})
}

func newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t *testing.T, options httpinterface.RouterOptions, mutationExecutor application.MutationExecutor) http.Handler {
	return newPolicyHTTPHandlerWithExecutors(t, options, memoryQueryExecutor{}, mutationExecutor)
}

func newPolicyHTTPHandlerWithExecutors(t *testing.T, options httpinterface.RouterOptions, queryExecutor application.QueryExecutor, mutationExecutor application.MutationExecutor) http.Handler {
	t.Helper()
	adapter := newMemorySnapshotAdapter(queryExecutor, mutationExecutor)
	queryPolicies := application.NewQueryPolicyManagement(adapter, application.NewQueryPolicyTypeRegistry(), "test-operator")
	mutationPolicies := application.NewMutationPolicyManagement(adapter, application.NewMutationPolicyTypeRegistry(), "test-operator")
	policies := application.NewTablePolicyManagement(adapter, adapter, queryPolicies, mutationPolicies, "test-operator")
	queries := application.NewManagedTableQuery(adapter, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry())
	mutations := application.NewManagedTableMutation(adapter, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry(), application.NewFixedOperatorProvider("test-operator"))
	return httpinterface.NewRouter(application.NewDatabaseTableDiscovery(adapter), readyAdapter{}, queryPolicies, mutationPolicies, policies, queries, mutations, options)
}

func validPolicyPayload(tableName string) string {
	return `{"table_name":"` + tableName + `","query_policy_code":"test_page_query_v1","mutation_policy_code":"test_mutation_v1"}`
}

func performRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

type memorySnapshotAdapter struct {
	mu               sync.Mutex
	tables           map[string]domain.DatabaseTable
	tablePolicies    map[string]domain.TablePolicy
	queryPolicies    map[string]domain.QueryPolicy
	mutationPolicies map[string]domain.MutationPolicy
	queryExecutor    application.QueryExecutor
	mutationExecutor application.MutationExecutor
}

func newMemorySnapshotAdapter(queryExecutor application.QueryExecutor, mutationExecutor application.MutationExecutor) *memorySnapshotAdapter {
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
		queryExecutor: queryExecutor, mutationExecutor: mutationExecutor,
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
func (adapter *memorySnapshotAdapter) ExecuteMutationSnapshot(ctx context.Context, execute func(application.MutationSnapshotSession) error) error {
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
func (session *memorySnapshotSession) DatabaseTime(context.Context) (time.Time, error) {
	return time.Now().UTC(), nil
}
func (session *memorySnapshotSession) InsertRow(ctx context.Context, insert domain.RowInsert) (string, error) {
	return session.mutationExecutor.InsertRow(ctx, insert)
}
func (session *memorySnapshotSession) UpdateRow(ctx context.Context, update domain.RowUpdate) (int64, error) {
	return session.mutationExecutor.UpdateRow(ctx, update)
}
func (session *memorySnapshotSession) DeleteRow(ctx context.Context, deletion domain.RowDelete) (int64, error) {
	return session.mutationExecutor.DeleteRow(ctx, deletion)
}

type readyAdapter struct{}

func (readyAdapter) Ready(context.Context) error { return nil }

type memoryQueryExecutor struct{}

func (memoryQueryExecutor) ExecutePageQuery(context.Context, domain.PageQuery) (domain.QueryResult, error) {
	return domain.QueryResult{}, nil
}

type memoryMutationExecutor struct{}

func (memoryMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	return "42", nil
}
func (memoryMutationExecutor) UpdateRow(context.Context, domain.RowUpdate) (int64, error) {
	return 1, nil
}
func (memoryMutationExecutor) DeleteRow(context.Context, domain.RowDelete) (int64, error) {
	return 1, nil
}

type panicMutationExecutor struct{}

func (panicMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	panic("mutation executor panic")
}
func (panicMutationExecutor) UpdateRow(context.Context, domain.RowUpdate) (int64, error) {
	panic("mutation executor panic")
}
func (panicMutationExecutor) DeleteRow(context.Context, domain.RowDelete) (int64, error) {
	panic("mutation executor panic")
}

var _ application.QuerySnapshotExecutor = (*memorySnapshotAdapter)(nil)
var _ application.MutationSnapshotExecutor = (*memorySnapshotAdapter)(nil)
var _ domain.TablePolicyCatalog = (*memorySnapshotAdapter)(nil)
var _ domain.QueryPolicyCatalog = (*memorySnapshotAdapter)(nil)
var _ domain.MutationPolicyCatalog = (*memorySnapshotAdapter)(nil)
