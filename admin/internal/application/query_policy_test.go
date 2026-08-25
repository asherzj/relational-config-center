package application

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestQueryPolicyLifecycleAndAssignmentRules(t *testing.T) {
	catalog := &memoryQueryPolicyCatalog{policies: make(map[string]domain.QueryPolicy)}
	management := NewQueryPolicyManagement(catalog, NewQueryPolicyTypeRegistry(), "test-operator")
	ctx := context.Background()
	candidate := validPutQueryPolicy("standard_page_query_v1")

	created, err := management.Create(ctx, candidate)
	if err != nil || created.Status != domain.PolicyStatusDraft {
		t.Fatalf("create Draft: policy=%#v err=%v", created, err)
	}
	if _, err := management.GetForNewAssignment(ctx, candidate.Code); !errors.Is(err, ErrQueryPolicyNotAssignable) {
		t.Fatalf("Draft should not be assignable, got %v", err)
	}
	active, err := management.Activate(ctx, candidate.Code)
	if err != nil || active.Status != domain.PolicyStatusActive {
		t.Fatalf("activate: policy=%#v err=%v", active, err)
	}
	if _, err := management.GetForNewAssignment(ctx, candidate.Code); err != nil {
		t.Fatalf("Active should be assignable: %v", err)
	}
	if _, err := management.ReplaceDraft(ctx, candidate.Code, candidate); !errors.Is(err, domain.ErrQueryPolicyStateConflict) {
		t.Fatalf("Active execution rules should be immutable, got %v", err)
	}
	deprecated, err := management.Deprecate(ctx, candidate.Code)
	if err != nil || deprecated.Status != domain.PolicyStatusDeprecated {
		t.Fatalf("deprecate: policy=%#v err=%v", deprecated, err)
	}
	if _, err := management.GetForNewAssignment(ctx, candidate.Code); !errors.Is(err, ErrQueryPolicyNotAssignable) {
		t.Fatalf("Deprecated should reject new assignments, got %v", err)
	}
	if _, err := management.Activate(ctx, candidate.Code); !errors.Is(err, ErrInvalidPolicyTransition) {
		t.Fatalf("Deprecated should not reactivate, got %v", err)
	}
	updated, err := management.UpdateMetadata(ctx, candidate.Code, "Renamed", "display only")
	if err != nil || updated.Name != "Renamed" || updated.DefaultPageSize != candidate.DefaultPageSize {
		t.Fatalf("metadata update: policy=%#v err=%v", updated, err)
	}
	if err := management.DeleteDraft(ctx, candidate.Code); !errors.Is(err, ErrInvalidPolicyTransition) {
		t.Fatalf("Deprecated should not be deleted, got %v", err)
	}
}

func TestQueryPolicyActivationValidatesRegisteredTypeAndSafetyLimits(t *testing.T) {
	registry := NewQueryPolicyTypeRegistry()
	if types := registry.List(); len(types) != 1 || types[0].Code != PageQueryPolicyType {
		t.Fatalf("unexpected Type registry: %#v", types)
	}
	base := domain.QueryPolicy{TypeCode: PageQueryPolicyType, DefaultOrderField: "id", DefaultOrderDirection: "DESC", DefaultPageSize: 20, MaxPageSize: 200}
	tests := []struct {
		name   string
		change func(*domain.QueryPolicy)
		want   error
	}{
		{name: "unknown Type", change: func(policy *domain.QueryPolicy) { policy.TypeCode = "dynamic_plugin" }, want: ErrUnknownQueryPolicyType},
		{name: "unsafe field", change: func(policy *domain.QueryPolicy) { policy.DefaultOrderField = "id DESC; DROP" }, want: ErrInvalidQueryPolicyRules},
		{name: "invalid direction", change: func(policy *domain.QueryPolicy) { policy.DefaultOrderDirection = "desc" }, want: ErrInvalidQueryPolicyRules},
		{name: "zero page size", change: func(policy *domain.QueryPolicy) { policy.DefaultPageSize = 0 }, want: ErrInvalidQueryPolicyRules},
		{name: "inverted sizes", change: func(policy *domain.QueryPolicy) { policy.DefaultPageSize = 30; policy.MaxPageSize = 20 }, want: ErrInvalidQueryPolicyRules},
		{name: "platform limit", change: func(policy *domain.QueryPolicy) { policy.MaxPageSize = 201 }, want: ErrInvalidQueryPolicyRules},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := base
			test.change(&policy)
			if err := registry.Validate(policy); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestQueryPolicyDraftRejectsValuesBlockedByCatalogScalarConstraints(t *testing.T) {
	tests := []struct {
		name   string
		change func(*PutQueryPolicy)
	}{
		{name: "invalid direction", change: func(policy *PutQueryPolicy) { policy.DefaultOrderDirection = "desc" }},
		{name: "zero default size", change: func(policy *PutQueryPolicy) { policy.DefaultPageSize = 0 }},
		{name: "zero max size", change: func(policy *PutQueryPolicy) { policy.MaxPageSize = 0 }},
		{name: "inverted sizes", change: func(policy *PutQueryPolicy) { policy.DefaultPageSize = 30; policy.MaxPageSize = 20 }},
		{name: "maximum above limit", change: func(policy *PutQueryPolicy) { policy.MaxPageSize = 201 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := &memoryQueryPolicyCatalog{policies: make(map[string]domain.QueryPolicy)}
			management := NewQueryPolicyManagement(catalog, NewQueryPolicyTypeRegistry(), "operator")
			candidate := validPutQueryPolicy("constrained_page_query_v1")
			test.change(&candidate)
			if _, err := management.Create(context.Background(), candidate); !errors.Is(err, ErrInvalidQueryPolicyRules) {
				t.Fatalf("expected stable scalar validation error, got %v", err)
			}
			if len(catalog.policies) != 0 {
				t.Fatalf("invalid Draft reached the Catalog: %#v", catalog.policies)
			}
		})
	}
}

func TestQueryPolicyCodeIsVersionedLowerCaseAndTechnologyNeutral(t *testing.T) {
	management := NewQueryPolicyManagement(&memoryQueryPolicyCatalog{policies: make(map[string]domain.QueryPolicy)}, NewQueryPolicyTypeRegistry(), "operator")
	for _, code := range []string{"standard_page_query", "Standard_page_query_v1", "mysql_page_query_v1", "postgres_page_query_v2", "standard-page-query-v1", "standard_page_query_v0"} {
		candidate := validPutQueryPolicy(code)
		if _, err := management.Create(context.Background(), candidate); !errors.Is(err, ErrInvalidPolicyCode) {
			t.Errorf("code %q: expected ErrInvalidPolicyCode, got %v", code, err)
		}
	}
}

func validPutQueryPolicy(code string) PutQueryPolicy {
	return PutQueryPolicy{Code: code, Name: "Standard", Description: "Reusable", TypeCode: PageQueryPolicyType, DefaultOrderField: "id", DefaultOrderDirection: "DESC", DefaultPageSize: 20, MaxPageSize: 200}
}

type memoryQueryPolicyCatalog struct {
	policies map[string]domain.QueryPolicy
}

func (catalog *memoryQueryPolicyCatalog) CreateQueryPolicy(_ context.Context, policy domain.QueryPolicy, operator string) (domain.QueryPolicy, error) {
	if _, found := catalog.policies[policy.Code]; found {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyExists
	}
	now := time.Now().UTC()
	policy.Creator, policy.Modifier, policy.CreatedAt, policy.UpdatedAt = operator, operator, now, now
	catalog.policies[policy.Code] = policy
	return policy, nil
}

func (catalog *memoryQueryPolicyCatalog) ListQueryPolicies(context.Context) ([]domain.QueryPolicy, error) {
	result := make([]domain.QueryPolicy, 0, len(catalog.policies))
	for _, policy := range catalog.policies {
		result = append(result, policy)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result, nil
}

func (catalog *memoryQueryPolicyCatalog) GetQueryPolicy(_ context.Context, code string) (domain.QueryPolicy, error) {
	policy, found := catalog.policies[code]
	if !found {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyNotFound
	}
	return policy, nil
}

func (catalog *memoryQueryPolicyCatalog) ReplaceDraftQueryPolicy(_ context.Context, replacement domain.QueryPolicy, operator string) (domain.QueryPolicy, error) {
	current, found := catalog.policies[replacement.Code]
	if !found {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyNotFound
	}
	if current.Status != domain.PolicyStatusDraft {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyStateConflict
	}
	replacement.Creator, replacement.CreatedAt, replacement.Modifier, replacement.UpdatedAt = current.Creator, current.CreatedAt, operator, time.Now().UTC()
	catalog.policies[replacement.Code] = replacement
	return replacement, nil
}

func (catalog *memoryQueryPolicyCatalog) SetQueryPolicyStatus(_ context.Context, code string, from, to domain.PolicyStatus, operator string) (domain.QueryPolicy, error) {
	policy, found := catalog.policies[code]
	if !found {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyNotFound
	}
	if policy.Status != from {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyStateConflict
	}
	policy.Status, policy.Modifier, policy.UpdatedAt = to, operator, time.Now().UTC()
	catalog.policies[code] = policy
	return policy, nil
}

func (catalog *memoryQueryPolicyCatalog) UpdateQueryPolicyMetadata(_ context.Context, code, name, description, operator string) (domain.QueryPolicy, error) {
	policy, found := catalog.policies[code]
	if !found {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyNotFound
	}
	if policy.Status == domain.PolicyStatusDraft {
		return domain.QueryPolicy{}, domain.ErrQueryPolicyStateConflict
	}
	policy.Name, policy.Description, policy.Modifier, policy.UpdatedAt = name, description, operator, time.Now().UTC()
	catalog.policies[code] = policy
	return policy, nil
}

func (catalog *memoryQueryPolicyCatalog) DeleteDraftQueryPolicy(_ context.Context, code string) error {
	policy, found := catalog.policies[code]
	if !found {
		return domain.ErrQueryPolicyNotFound
	}
	if policy.Status != domain.PolicyStatusDraft {
		return domain.ErrQueryPolicyStateConflict
	}
	delete(catalog.policies, code)
	return nil
}
