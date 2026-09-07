package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestTransactionalManagedTableMutationUsesOneCompleteSnapshotAndDatabaseAutoFill(t *testing.T) {
	session := validMutationSnapshotSession()
	mutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: session}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())

	id, err := mutation.Add((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", domain.MutationContent{"name": jsonStringPointer("created")})
	if err != nil {
		t.Fatalf("ADD through relational Policy Snapshot: %v", err)
	}
	if id != "41" {
		t.Fatalf("expected returned id 41, got %q", id)
	}
	wantCalls := []string{
		"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1", "schema:managed_items", "database-time", "insert:managed_items",
	}
	if !reflect.DeepEqual(session.calls, wantCalls) {
		t.Fatalf("expected separate Snapshot reads and one in-session write, got %#v", session.calls)
	}
	values := mutationValueMap(session.insert.Values)
	if values["name"] != "created" || values["creator"] != "00000000-0000-4000-8000-000000000001" || values["modifier"] != "00000000-0000-4000-8000-000000000001" {
		t.Fatalf("operator Auto Fill is incomplete: %#v", values)
	}
	wantTime := session.databaseTime.Truncate(time.Microsecond)
	if values["created_at"] != wantTime || values["updated_at"] != wantTime {
		t.Fatalf("ADD must use the same database time for Create and Modify targets: %#v", values)
	}
}

func TestTransactionalManagedTableMutationRejectsClientManagedFieldsInsteadOfOverwriting(t *testing.T) {
	session := validMutationSnapshotSession()
	mutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: session}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
	_, err := mutation.Add((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", domain.MutationContent{
		"name":    jsonStringPointer("created"),
		"creator": jsonStringPointer("client-value"),
	})
	if !errors.Is(err, ErrInvalidMutation) {
		t.Fatalf("expected configured server-managed field rejection, got %v", err)
	}
	if containsCall(session.calls, "database-time") || containsCall(session.calls, "insert:managed_items") {
		t.Fatalf("rejected content reached Auto Fill or row execution: %#v", session.calls)
	}
}

func TestTransactionalManagedTableMutationAuthorizationComesOnlyFromMutationPolicy(t *testing.T) {
	session := validMutationSnapshotSession()
	session.mutationPolicy.AllowAdd = false
	session.mutationPolicy.CreateOperatorField = nil
	session.mutationPolicy.CreateTimeField = nil
	mutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: session}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
	_, err := mutation.Add((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", domain.MutationContent{"name": jsonStringPointer("denied")})
	if !errors.Is(err, ErrMutationNotAllowed) {
		t.Fatalf("expected reusable Mutation Policy to deny ADD, got %v", err)
	}
	if containsCall(session.calls, "insert:managed_items") {
		t.Fatalf("denied ADD reached row execution: %#v", session.calls)
	}
}

func TestTransactionalManagedTableMutationModifyFillsOnlyModifyAndDeleteFillsNothing(t *testing.T) {
	modifySession := validMutationSnapshotSession()
	mutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: modifySession}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
	if _, err := mutation.Modify((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", "7", domain.MutationContent{"name": jsonStringPointer("changed")}); err != nil {
		t.Fatalf("MODIFY through relational Policy Snapshot: %v", err)
	}
	values := mutationValueMap(modifySession.update.Values)
	if _, found := values["creator"]; found {
		t.Fatalf("MODIFY changed Create Operator: %#v", values)
	}
	if _, found := values["created_at"]; found {
		t.Fatalf("MODIFY changed Create Time: %#v", values)
	}
	if values["modifier"] != "00000000-0000-4000-8000-000000000001" || values["updated_at"] != modifySession.databaseTime.Truncate(time.Microsecond) {
		t.Fatalf("MODIFY did not fill Modify targets: %#v", values)
	}

	deleteSession := validMutationSnapshotSession()
	deleteMutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: deleteSession}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
	if _, err := deleteMutation.Delete((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", "7"); err != nil {
		t.Fatalf("DELETE through relational Policy Snapshot: %v", err)
	}
	if containsCall(deleteSession.calls, "database-time") {
		t.Fatalf("DELETE unexpectedly produced Auto Fill: %#v", deleteSession.calls)
	}
}

func TestTransactionalManagedTableMutationAllowsDeprecatedAndFailsClosedForDraftOrUnknownTypes(t *testing.T) {
	deprecated := validMutationSnapshotSession()
	deprecated.queryPolicy.Status = domain.PolicyStatusDeprecated
	deprecated.mutationPolicy.Status = domain.PolicyStatusDeprecated
	mutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: deprecated}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
	if _, err := mutation.Delete((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", "7"); err != nil {
		t.Fatalf("existing Deprecated references must execute: %v", err)
	}

	tests := []struct {
		name string
		edit func(*memoryMutationSnapshotSession)
		want error
	}{
		{name: "Draft Query Policy", edit: func(session *memoryMutationSnapshotSession) { session.queryPolicy.Status = domain.PolicyStatusDraft }, want: ErrInvalidPolicySnapshot},
		{name: "Draft Mutation Policy", edit: func(session *memoryMutationSnapshotSession) { session.mutationPolicy.Status = domain.PolicyStatusDraft }, want: ErrInvalidPolicySnapshot},
		{name: "unknown Query Type", edit: func(session *memoryMutationSnapshotSession) { session.queryPolicy.TypeCode = "unknown" }, want: ErrUnknownQueryPolicyType},
		{name: "unknown Mutation Type", edit: func(session *memoryMutationSnapshotSession) { session.mutationPolicy.TypeCode = "unknown" }, want: ErrUnknownMutationPolicyType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := validMutationSnapshotSession()
			test.edit(session)
			mutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: session}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
			if _, err := mutation.Delete((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", "7"); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if containsCall(session.calls, "delete:managed_items") {
				t.Fatalf("invalid Snapshot reached row execution: %#v", session.calls)
			}
		})
	}
}

type memoryMutationSnapshotExecutor struct {
	session       *memoryMutationSnapshotSession
	callbackError error
}

func (executor *memoryMutationSnapshotExecutor) ExecuteMutationSnapshot(_ context.Context, execute func(MutationSnapshotSession) error) error {
	executor.callbackError = execute(executor.session)
	return executor.callbackError
}

type memoryMutationSnapshotSession struct {
	tablePolicy    domain.TablePolicy
	queryPolicy    domain.QueryPolicy
	mutationPolicy domain.MutationPolicy
	schema         domain.TableSchema
	databaseTime   time.Time
	calls          []string
	insert         domain.RowInsert
	insertID       *string
	update         domain.RowUpdate
	deletion       domain.RowDelete
}

func validMutationSnapshotSession() *memoryMutationSnapshotSession {
	createOperator, createTime := "creator", "created_at"
	modifyOperator, modifyTime := "modifier", "updated_at"
	return &memoryMutationSnapshotSession{
		tablePolicy: domain.TablePolicy{
			TableName: "managed_items", QueryPolicyCode: "standard_page_query_v1", MutationPolicyCode: "standard_mutation_v1", Enabled: true,
		},
		queryPolicy: domain.QueryPolicy{
			Code: "standard_page_query_v1", TypeCode: PageQueryPolicyType,
			DefaultOrderField: "id", DefaultOrderDirection: "ASC", DefaultPageSize: 20, MaxPageSize: 200, Status: domain.PolicyStatusActive,
		},
		mutationPolicy: domain.MutationPolicy{
			Code: "standard_mutation_v1", TypeCode: SingleTableMutationPolicyType,
			AllowAdd: true, AllowModify: true, AllowDelete: true,
			CreateOperatorField: &createOperator, CreateTimeField: &createTime,
			ModifyOperatorField: &modifyOperator, ModifyTimeField: &modifyTime,
			Status: domain.PolicyStatusActive,
		},
		schema: domain.TableSchema{
			Name: "managed_items", Compatible: true,
			Columns: []domain.Column{
				{Name: "id", Type: domain.ColumnTypeUInt64, AutoIncrement: true},
				{Name: "name", Type: domain.ColumnTypeString, TextCapacity: 100},
				{Name: "creator", Type: domain.ColumnTypeString, TextCapacity: 100},
				{Name: "created_at", Type: domain.ColumnTypeDateTime},
				{Name: "modifier", Type: domain.ColumnTypeString, TextCapacity: 100},
				{Name: "updated_at", Type: domain.ColumnTypeDateTime},
			},
		},
		databaseTime: time.Date(2026, 8, 24, 10, 11, 12, 123456000, time.UTC),
	}
}

func (session *memoryMutationSnapshotSession) GetTablePolicy(_ context.Context, tableName string) (domain.TablePolicy, error) {
	session.calls = append(session.calls, "table:"+tableName)
	return session.tablePolicy, nil
}

func (session *memoryMutationSnapshotSession) GetQueryPolicy(_ context.Context, code string) (domain.QueryPolicy, error) {
	session.calls = append(session.calls, "query:"+code)
	return session.queryPolicy, nil
}

func (session *memoryMutationSnapshotSession) GetMutationPolicy(_ context.Context, code string) (domain.MutationPolicy, error) {
	session.calls = append(session.calls, "mutation:"+code)
	return session.mutationPolicy, nil
}

func (session *memoryMutationSnapshotSession) GetTableSchema(_ context.Context, tableName string) (domain.TableSchema, error) {
	session.calls = append(session.calls, "schema:"+tableName)
	return session.schema, nil
}

func (session *memoryMutationSnapshotSession) DatabaseTime(context.Context) (time.Time, error) {
	session.calls = append(session.calls, "database-time")
	return session.databaseTime, nil
}

func (session *memoryMutationSnapshotSession) InsertRow(_ context.Context, insert domain.RowInsert) (string, error) {
	session.calls = append(session.calls, "insert:"+insert.TableName)
	session.insert = insert
	if session.insertID != nil {
		return *session.insertID, nil
	}
	return "41", nil
}

func TestManagedTableAddRejectsUnaddressableRawAndCanonicalIdentities(t *testing.T) {
	for _, test := range []struct {
		name, raw, canonical string
		wantInsert           bool
	}{
		{name: "empty input", raw: "", canonical: "41"},
		{name: "dot input", raw: ".", canonical: "41"},
		{name: "double dot input", raw: "..", canonical: "41"},
		{name: "empty returned identity", raw: "valid", canonical: "", wantInsert: true},
		{name: "normalized dot", raw: ".  ", canonical: ".", wantInsert: true},
		{name: "normalized double dot", raw: "..  ", canonical: "..", wantInsert: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := validMutationSnapshotSession()
			session.schema.Columns[0] = domain.Column{Name: "id", Type: domain.ColumnTypeString}
			session.insertID = &test.canonical
			executor := &memoryMutationSnapshotExecutor{session: session}
			mutation := NewManagedTableMutation(executor, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
			_, err := mutation.Add((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", domain.MutationContent{"id": jsonStringPointer(test.raw), "name": jsonStringPointer("test")})
			if !errors.Is(err, ErrInvalidMutation) || !errors.Is(executor.callbackError, ErrInvalidMutation) {
				t.Fatalf("identity rejection must occur inside the transaction callback: error=%v callback=%v", err, executor.callbackError)
			}
			if got := containsCall(session.calls, "insert:managed_items"); got != test.wantInsert {
				t.Fatalf("INSERT reached=%v, want %v", got, test.wantInsert)
			}
		})
	}
}

func TestManagedTableAddValidatesGeneratedIdentityAgainstSnapshotSchema(t *testing.T) {
	for _, id := range []string{"1", "2"} {
		t.Run(id, func(t *testing.T) {
			session := validMutationSnapshotSession()
			session.schema.Columns[0] = domain.Column{Name: "id", Type: domain.ColumnTypeBoolean, AutoIncrement: true}
			session.insertID = &id
			executor := &memoryMutationSnapshotExecutor{session: session}
			mutation := NewManagedTableMutation(executor, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
			result, err := mutation.Add((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", domain.MutationContent{"name": jsonStringPointer("test")})
			if id == "1" {
				if err != nil || result != id {
					t.Fatalf("valid generated identity: id=%q error=%v", result, err)
				}
			} else if !errors.Is(err, ErrInvalidMutation) || !errors.Is(executor.callbackError, ErrInvalidMutation) {
				t.Fatalf("unaddressable generated identity must reject inside the transaction callback: error=%v callback=%v", err, executor.callbackError)
			}
			if !containsCall(session.calls, "insert:managed_items") {
				t.Fatal("generated identity must be validated after insertion")
			}
		})
	}
}

func (session *memoryMutationSnapshotSession) UpdateRow(_ context.Context, update domain.RowUpdate) (int64, error) {
	session.calls = append(session.calls, "update:"+update.TableName)
	session.update = update
	return 1, nil
}

func (session *memoryMutationSnapshotSession) DeleteRow(_ context.Context, deletion domain.RowDelete) (int64, error) {
	session.calls = append(session.calls, "delete:"+deletion.TableName)
	session.deletion = deletion
	return 1, nil
}

func mutationValueMap(values []domain.MutationValue) map[string]any {
	result := make(map[string]any, len(values))
	for _, value := range values {
		result[value.Column.Name] = value.Value
	}
	return result
}

func jsonStringPointer(value string) *domain.JSONString {
	converted := domain.JSONString(value)
	return &converted
}

func containsCall(calls []string, wanted string) bool {
	for _, call := range calls {
		if call == wanted {
			return true
		}
	}
	return false
}

var _ MutationSnapshotExecutor = (*memoryMutationSnapshotExecutor)(nil)
var _ MutationSnapshotSession = (*memoryMutationSnapshotSession)(nil)

func TestNonAutoIncrementIDIsRequiredEvenWhenSchemaHasADefault(t *testing.T) {
	session := validMutationSnapshotSession()
	session.schema.Columns[0].AutoIncrement = false
	session.schema.Columns[0].HasDefault = true
	mutation := NewManagedTableMutation(&memoryMutationSnapshotExecutor{session: session}, NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
	_, err := mutation.Add((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", domain.MutationContent{"name": jsonStringPointer("created")})
	if !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("missing non-auto id must fail before insert even with a default: %v", err)
	}
	if containsCall(session.calls, "insert:managed_items") {
		t.Fatalf("unknown default identity reached row execution: %#v", session.calls)
	}
	_, err = mutation.Add((AuthenticatedOperator{accountID: "00000000-0000-4000-8000-000000000001"}).Bind(t.Context()), "managed_items", domain.MutationContent{"id": jsonStringPointer("42"), "name": jsonStringPointer("created")})
	if err != nil || session.insert.ProvidedID == nil || *session.insert.ProvidedID != "42" {
		t.Fatalf("explicit non-auto id must remain supported: id=%v error=%v", session.insert.ProvidedID, err)
	}
}
