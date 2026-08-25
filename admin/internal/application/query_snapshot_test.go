package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestTransactionalManagedTableQueryLoadsAndExecutesCompleteRelationalSnapshotPerRequest(t *testing.T) {
	session := validQuerySnapshotSession()
	executor := &memoryQuerySnapshotExecutor{session: session}
	query := NewManagedTableQuery(executor, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())

	for range 2 {
		result, err := query.Execute(t.Context(), "managed_items", domain.QuerySpec{PageNumber: 2})
		if err != nil {
			t.Fatalf("execute relational Policy Snapshot: %v", err)
		}
		if result.Page.PageSize != 2 {
			t.Fatalf("expected relational default page size 2, got %#v", result.Page)
		}
	}
	wantCalls := []string{
		"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1", "schema:managed_items", "execute:managed_items",
		"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1", "schema:managed_items", "execute:managed_items",
	}
	if !reflect.DeepEqual(session.calls, wantCalls) {
		t.Fatalf("expected separate uncached Snapshot reads per request, got %#v", session.calls)
	}
	for _, pageQuery := range session.queries {
		if pageQuery.Order.Field != "id" || pageQuery.Order.Direction != "ASC" || pageQuery.PageNumber != 2 || pageQuery.PageSize != 2 || pageQuery.Offset != 2 {
			t.Fatalf("executor did not receive relational ordering/pagination values: %#v", pageQuery)
		}
	}
}

func TestTransactionalManagedTableQueryAllowsAssignedDeprecatedDefinitions(t *testing.T) {
	session := validQuerySnapshotSession()
	session.queryPolicy.Status = domain.PolicyStatusDeprecated
	session.mutationPolicy.Status = domain.PolicyStatusDeprecated
	query := NewManagedTableQuery(&memoryQuerySnapshotExecutor{session: session}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
	if _, err := query.Execute(t.Context(), "managed_items", domain.QuerySpec{}); err != nil {
		t.Fatalf("assigned Deprecated definitions must execute: %v", err)
	}
}

func TestTransactionalManagedTableQueryFailsClosedBeforeDataExecution(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*memoryQuerySnapshotSession)
		want      error
		wantCalls []string
	}{
		{
			name:      "disabled assignment still reads complete definitions",
			edit:      func(session *memoryQuerySnapshotSession) { session.tablePolicy.Enabled = false },
			want:      ErrTablePolicyDisabled,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "Draft Query Policy",
			edit:      func(session *memoryQuerySnapshotSession) { session.queryPolicy.Status = domain.PolicyStatusDraft },
			want:      ErrInvalidPolicySnapshot,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "unknown Query Type",
			edit:      func(session *memoryQuerySnapshotSession) { session.queryPolicy.TypeCode = "unknown" },
			want:      ErrUnknownQueryPolicyType,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "unknown Mutation Type",
			edit:      func(session *memoryQuerySnapshotSession) { session.mutationPolicy.TypeCode = "unknown" },
			want:      ErrUnknownMutationPolicyType,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "Policy cannot relax code-owned page maximum",
			edit:      func(session *memoryQuerySnapshotSession) { session.queryPolicy.MaxPageSize = 500 },
			want:      ErrInvalidQueryPolicyRules,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "incompatible live default order",
			edit:      func(session *memoryQuerySnapshotSession) { session.queryPolicy.DefaultOrderField = "missing" },
			want:      ErrInvalidQueryPolicyRules,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1", "schema:managed_items"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := validQuerySnapshotSession()
			test.edit(session)
			query := NewManagedTableQuery(&memoryQuerySnapshotExecutor{session: session}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
			if _, err := query.Execute(t.Context(), "managed_items", domain.QuerySpec{}); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if !reflect.DeepEqual(session.calls, test.wantCalls) {
				t.Fatalf("unexpected operations before fail-closed result: %#v", session.calls)
			}
		})
	}
}

type memoryQuerySnapshotExecutor struct {
	session *memoryQuerySnapshotSession
}

func (executor *memoryQuerySnapshotExecutor) ExecuteQuerySnapshot(_ context.Context, execute func(QuerySnapshotSession) (domain.QueryResult, error)) (domain.QueryResult, error) {
	return execute(executor.session)
}

type memoryQuerySnapshotSession struct {
	tablePolicy    domain.TablePolicy
	queryPolicy    domain.QueryPolicy
	mutationPolicy domain.MutationPolicy
	schema         domain.TableSchema
	calls          []string
	queries        []domain.PageQuery
}

func validQuerySnapshotSession() *memoryQuerySnapshotSession {
	return &memoryQuerySnapshotSession{
		tablePolicy: domain.TablePolicy{
			TableName: "managed_items", QueryPolicyCode: "standard_page_query_v1", MutationPolicyCode: "standard_mutation_v1", Enabled: true,
		},
		queryPolicy: domain.QueryPolicy{
			Code: "standard_page_query_v1", TypeCode: PageQueryPolicyType,
			DefaultOrderField: "id", DefaultOrderDirection: "ASC", DefaultPageSize: 2, MaxPageSize: 5, Status: domain.PolicyStatusActive,
		},
		mutationPolicy: domain.MutationPolicy{
			Code: "standard_mutation_v1", TypeCode: SingleTableMutationPolicyType, Status: domain.PolicyStatusActive,
		},
		schema: domain.TableSchema{
			Name: "managed_items", Compatible: true,
			Columns: []domain.Column{{Name: "id", Type: domain.ColumnTypeUInt64, AutoIncrement: true}, {Name: "name", Type: domain.ColumnTypeString}},
		},
	}
}

func (session *memoryQuerySnapshotSession) GetTablePolicy(_ context.Context, tableName string) (domain.TablePolicy, error) {
	session.calls = append(session.calls, "table:"+tableName)
	return session.tablePolicy, nil
}

func (session *memoryQuerySnapshotSession) GetQueryPolicy(_ context.Context, code string) (domain.QueryPolicy, error) {
	session.calls = append(session.calls, "query:"+code)
	return session.queryPolicy, nil
}

func (session *memoryQuerySnapshotSession) GetMutationPolicy(_ context.Context, code string) (domain.MutationPolicy, error) {
	session.calls = append(session.calls, "mutation:"+code)
	return session.mutationPolicy, nil
}

func (session *memoryQuerySnapshotSession) GetTableSchema(_ context.Context, tableName string) (domain.TableSchema, error) {
	session.calls = append(session.calls, "schema:"+tableName)
	return session.schema, nil
}

func (session *memoryQuerySnapshotSession) ExecutePageQuery(_ context.Context, query domain.PageQuery) (domain.QueryResult, error) {
	session.calls = append(session.calls, "execute:"+query.TableName)
	session.queries = append(session.queries, query)
	return domain.QueryResult{Page: domain.Page{PageNumber: query.PageNumber, PageSize: query.PageSize}}, nil
}

var _ QuerySnapshotExecutor = (*memoryQuerySnapshotExecutor)(nil)
var _ QuerySnapshotSession = (*memoryQuerySnapshotSession)(nil)
