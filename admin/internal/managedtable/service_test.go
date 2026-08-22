package managedtable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type recordingRepository struct {
	query       QuerySpec
	created     map[string]any
	updated     map[string]any
	updatedKey  any
	deletedKey  any
	queryCalls  int
	createCalls int
	updateCalls int
	deleteCalls int
}

func (repository *recordingRepository) Query(_ context.Context, _ Policy, query QuerySpec) (PageResult, error) {
	repository.queryCalls++
	repository.query = query
	return PageResult{Rows: []map[string]any{}, Page: PageInfo{Number: query.Page.Number, Size: query.Page.Size}}, nil
}

func (repository *recordingRepository) Create(_ context.Context, _ Policy, values map[string]any) (MutationResult, error) {
	repository.createCalls++
	repository.created = values
	return MutationResult{AffectedRows: 1, Key: "1"}, nil
}

func (repository *recordingRepository) Update(_ context.Context, _ Policy, key any, values map[string]any) (MutationResult, error) {
	repository.updateCalls++
	repository.updatedKey = key
	repository.updated = values
	return MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
}

func (repository *recordingRepository) Delete(_ context.Context, _ Policy, key any) (MutationResult, error) {
	repository.deleteCalls++
	repository.deletedKey = key
	return MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
}

func TestServicePreparesQueryBeforeRepository(t *testing.T) {
	repository := &recordingRepository{}
	service := testService(t, repository)
	query := QuerySpec{Filter: &Filter{
		Logic: LogicAnd,
		Items: []Filter{
			{Field: "name", Operator: OperatorContains, Value: "prod"},
			{Field: "id", Operator: OperatorIn, Value: []any{json.Number("1"), json.Number("2")}},
		},
	}}

	result, err := service.Query(context.Background(), "widgets", query)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.Page.Number != 1 || result.Page.Size != 20 {
		t.Fatalf("Query() page = %+v, want defaults", result.Page)
	}
	if repository.queryCalls != 1 {
		t.Fatalf("repository query calls = %d, want 1", repository.queryCalls)
	}
	if got := repository.query.Sort; !reflect.DeepEqual(got, []Sort{{Field: "id", Direction: DirectionAscending}}) {
		t.Fatalf("prepared sort = %#v", got)
	}
	values := repository.query.Filter.Items[1].Value.([]any)
	if _, ok := values[0].(uint64); !ok {
		t.Fatalf("IN value type = %T, want uint64", values[0])
	}
}

func TestServiceRejectsUnregisteredQueryFieldBeforeRepository(t *testing.T) {
	repository := &recordingRepository{}
	service := testService(t, repository)
	_, err := service.Query(context.Background(), "widgets", QuerySpec{Filter: &Filter{
		Field:    "name DESC; DROP TABLE widgets",
		Operator: OperatorEqual,
		Value:    "x",
	}})
	if !IsValidation(err) {
		t.Fatalf("Query() error = %v, want validation error", err)
	}
	if repository.queryCalls != 0 {
		t.Fatalf("repository query calls = %d, want 0", repository.queryCalls)
	}
}

func TestServiceEnforcesQueryComplexityLimits(t *testing.T) {
	repository := &recordingRepository{}
	service := testService(t, repository)
	_, err := service.Query(context.Background(), "widgets", QuerySpec{Page: Page{Number: 1, Size: 101}})
	if !IsValidation(err) {
		t.Fatalf("oversized page error = %v", err)
	}

	values := make([]any, 101)
	for index := range values {
		values[index] = json.Number("1")
	}
	_, err = service.Query(context.Background(), "widgets", QuerySpec{Filter: &Filter{
		Field: "id", Operator: OperatorIn, Value: values,
	}})
	if !IsValidation(err) {
		t.Fatalf("oversized IN error = %v", err)
	}
	if repository.queryCalls != 0 {
		t.Fatalf("repository query calls = %d, want 0", repository.queryCalls)
	}
}

func TestServiceValidatesMutationsAndPrimaryKey(t *testing.T) {
	repository := &recordingRepository{}
	service := testService(t, repository)

	_, err := service.Create(context.Background(), "widgets", map[string]any{"name": "first"})
	if !IsValidation(err) {
		t.Fatalf("Create() missing required field error = %v", err)
	}
	_, err = service.Create(context.Background(), "widgets", map[string]any{
		"name": "",
		"data": map[string]any{"enabled": true},
	})
	if !IsValidation(err) {
		t.Fatalf("Create() empty required name error = %v", err)
	}
	_, err = service.Create(context.Background(), "widgets", map[string]any{
		"name": "first",
		"data": map[string]any{"enabled": true},
		"id":   json.Number("1"),
	})
	if !IsValidation(err) {
		t.Fatalf("Create() read-only id error = %v", err)
	}

	created, err := service.Create(context.Background(), "widgets", map[string]any{
		"name": "first",
		"data": map[string]any{"enabled": true},
	})
	if err != nil || created.AffectedRows != 1 {
		t.Fatalf("Create() = %+v, %v", created, err)
	}
	if _, ok := repository.created["data"].(json.RawMessage); !ok {
		t.Fatalf("normalized JSON type = %T", repository.created["data"])
	}

	_, err = service.Update(context.Background(), "widgets", "not-a-number", map[string]any{"name": "renamed"})
	if !IsValidation(err) {
		t.Fatalf("Update() key error = %v", err)
	}
	updated, err := service.Update(context.Background(), "widgets", "42", map[string]any{"name": "renamed"})
	if err != nil || updated.AffectedRows != 1 {
		t.Fatalf("Update() = %+v, %v", updated, err)
	}
	if repository.updatedKey != uint64(42) {
		t.Fatalf("updated key = %#v", repository.updatedKey)
	}
}

func TestPolicyRejectsUnsafePrimaryKey(t *testing.T) {
	policy := testPolicy()
	policy.Fields["id"] = FieldPolicy{Column: "id", Type: TypeUnsigned, Readable: true}
	if err := policy.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want non-auto-increment key requirements error")
	}
}

func TestPolicyRejectsUnsafePhysicalIdentifier(t *testing.T) {
	policy := testPolicy()
	policy.Table = "app_widgets; DROP TABLE users"
	if err := policy.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe table identifier error")
	}
}

func TestServiceReportsUnknownResource(t *testing.T) {
	service := testService(t, &recordingRepository{})
	_, err := service.Describe("missing")
	if !errors.Is(err, ErrUnknownResource) {
		t.Fatalf("Describe() error = %v", err)
	}
}

func testService(t *testing.T, repository Repository) *Service {
	t.Helper()
	registry, err := NewRegistry(testPolicy())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return NewService(registry, repository)
}

func testPolicy() Policy {
	return Policy{
		Resource:    "widgets",
		Table:       "app_widgets",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		Fields: map[string]FieldPolicy{
			"id": {
				Column:          "widget_id",
				Type:            TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				AutoIncrement:   true,
				FilterOperators: []Operator{OperatorEqual, OperatorIn},
			},
			"name": {
				Column:           "widget_name",
				Type:             TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        32,
				FilterOperators:  []Operator{OperatorEqual, OperatorContains},
			},
			"data": {
				Column:           "widget_data",
				Type:             TypeJSON,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
			},
		},
	}
}
