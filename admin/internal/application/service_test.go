package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type recordingRepository struct {
	query       domain.QuerySpec
	created     map[string]any
	updated     map[string]any
	updatedKey  any
	deletedKey  any
	queryCalls  int
	createCalls int
	updateCalls int
	deleteCalls int
}

func (repository *recordingRepository) Query(_ context.Context, _ domain.Policy, query domain.QuerySpec) (domain.PageResult, error) {
	repository.queryCalls++
	repository.query = query
	return domain.PageResult{Rows: []map[string]any{}, Page: domain.PageInfo{Number: query.Page.Number, Size: query.Page.Size}}, nil
}

func (repository *recordingRepository) Create(_ context.Context, _ domain.Policy, values map[string]any) (domain.MutationResult, error) {
	repository.createCalls++
	repository.created = values
	return domain.MutationResult{AffectedRows: 1, Key: "1"}, nil
}

func (repository *recordingRepository) Update(_ context.Context, _ domain.Policy, key any, values map[string]any) (domain.MutationResult, error) {
	repository.updateCalls++
	repository.updatedKey = key
	repository.updated = values
	return domain.MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
}

func (repository *recordingRepository) Delete(_ context.Context, _ domain.Policy, key any) (domain.MutationResult, error) {
	repository.deleteCalls++
	repository.deletedKey = key
	return domain.MutationResult{AffectedRows: 1, Key: fmt.Sprint(key)}, nil
}

func TestServicePreparesQueryBeforeRepository(t *testing.T) {
	repository := &recordingRepository{}
	service := testService(t, repository)
	query := domain.QuerySpec{Filter: &domain.Filter{
		Logic: domain.LogicAnd,
		Items: []domain.Filter{
			{Field: "name", Operator: domain.OperatorContains, Value: "prod"},
			{Field: "id", Operator: domain.OperatorIn, Value: []any{json.Number("1"), json.Number("2")}},
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
	if got := repository.query.Sort; !reflect.DeepEqual(got, []domain.Sort{{Field: "id", Direction: domain.DirectionAscending}}) {
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
	_, err := service.Query(context.Background(), "widgets", domain.QuerySpec{Filter: &domain.Filter{
		Field:    "name DESC; DROP TABLE widgets",
		Operator: domain.OperatorEqual,
		Value:    "x",
	}})
	if !domain.IsValidation(err) {
		t.Fatalf("Query() error = %v, want validation error", err)
	}
	if repository.queryCalls != 0 {
		t.Fatalf("repository query calls = %d, want 0", repository.queryCalls)
	}
}

func TestServiceEnforcesQueryComplexityLimits(t *testing.T) {
	repository := &recordingRepository{}
	service := testService(t, repository)
	_, err := service.Query(context.Background(), "widgets", domain.QuerySpec{Page: domain.Page{Number: 1, Size: 101}})
	if !domain.IsValidation(err) {
		t.Fatalf("oversized page error = %v", err)
	}

	values := make([]any, 101)
	for index := range values {
		values[index] = json.Number("1")
	}
	_, err = service.Query(context.Background(), "widgets", domain.QuerySpec{Filter: &domain.Filter{
		Field: "id", Operator: domain.OperatorIn, Value: values,
	}})
	if !domain.IsValidation(err) {
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
	if !domain.IsValidation(err) {
		t.Fatalf("Create() missing required field error = %v", err)
	}
	_, err = service.Create(context.Background(), "widgets", map[string]any{
		"name": "",
		"data": map[string]any{"enabled": true},
	})
	if !domain.IsValidation(err) {
		t.Fatalf("Create() empty required name error = %v", err)
	}
	_, err = service.Create(context.Background(), "widgets", map[string]any{
		"name": "first",
		"data": map[string]any{"enabled": true},
		"id":   json.Number("1"),
	})
	if !domain.IsValidation(err) {
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
	if !domain.IsValidation(err) {
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

func TestServiceReportsUnknownResource(t *testing.T) {
	service := testService(t, &recordingRepository{})
	_, err := service.Describe("missing")
	if !errors.Is(err, domain.ErrUnknownResource) {
		t.Fatalf("Describe() error = %v", err)
	}
}

func testService(t *testing.T, repository domain.Repository) *Service {
	t.Helper()
	registry, err := domain.NewRegistry(testPolicy())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return NewService(NewPolicySource(registry), repository)
}

func testPolicy() domain.Policy {
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
				FilterOperators: []domain.Operator{domain.OperatorEqual, domain.OperatorIn},
			},
			"name": {
				Column:           "widget_name",
				Type:             domain.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        32,
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
