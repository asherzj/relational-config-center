package application

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestMutationPolicyLifecycleAndAssignmentRules(t *testing.T) {
	catalog := &memoryMutationPolicyCatalog{policies: make(map[string]domain.MutationPolicy)}
	management := NewMutationPolicyManagement(catalog, NewMutationPolicyTypeRegistry(), "test-operator")
	ctx := context.Background()
	candidate := validPutMutationPolicy("standard_mutation_v1")

	created, err := management.Create(ctx, candidate)
	if err != nil || created.Status != domain.PolicyStatusDraft {
		t.Fatalf("create Draft: policy=%#v err=%v", created, err)
	}
	if _, err := management.GetForNewAssignment(ctx, candidate.Code); !errors.Is(err, ErrMutationPolicyNotAssignable) {
		t.Fatalf("Draft should not be assignable, got %v", err)
	}
	if _, err := management.ReplaceDraft(ctx, "different_mutation_v1", candidate); !errors.Is(err, ErrInvalidPolicyCode) {
		t.Fatalf("Policy Code should be immutable, got %v", err)
	}
	active, err := management.Activate(ctx, candidate.Code)
	if err != nil || active.Status != domain.PolicyStatusActive {
		t.Fatalf("activate: policy=%#v err=%v", active, err)
	}
	if _, err := management.GetForNewAssignment(ctx, candidate.Code); err != nil {
		t.Fatalf("Active should be assignable: %v", err)
	}
	if _, err := management.ReplaceDraft(ctx, candidate.Code, candidate); !errors.Is(err, domain.ErrMutationPolicyStateConflict) {
		t.Fatalf("Active execution rules should be immutable, got %v", err)
	}
	deprecated, err := management.Deprecate(ctx, candidate.Code)
	if err != nil || deprecated.Status != domain.PolicyStatusDeprecated {
		t.Fatalf("deprecate: policy=%#v err=%v", deprecated, err)
	}
	if _, err := management.GetForNewAssignment(ctx, candidate.Code); !errors.Is(err, ErrMutationPolicyNotAssignable) {
		t.Fatalf("Deprecated should reject new assignments, got %v", err)
	}
	if _, err := management.Activate(ctx, candidate.Code); !errors.Is(err, ErrInvalidPolicyTransition) {
		t.Fatalf("Deprecated should not reactivate, got %v", err)
	}
	updated, err := management.UpdateMetadata(ctx, candidate.Code, "Renamed", "display only")
	if err != nil || updated.Name != "Renamed" || !updated.AllowAdd {
		t.Fatalf("metadata update: policy=%#v err=%v", updated, err)
	}
	if err := management.DeleteDraft(ctx, candidate.Code); !errors.Is(err, ErrInvalidPolicyTransition) {
		t.Fatalf("Deprecated should not be deleted, got %v", err)
	}
}

func TestMutationPolicyActivationValidatesTypeAutoFillAndAuthorization(t *testing.T) {
	registry := NewMutationPolicyTypeRegistry()
	types := registry.List()
	if len(types) != 1 || types[0].Code != SingleTableMutationPolicyType || !implementsAllMutationOperations(types[0].Operations) {
		t.Fatalf("unexpected Type registry: %#v", types)
	}
	base := mutationPolicyForValidation()
	tests := []struct {
		name   string
		change func(*domain.MutationPolicy)
		want   error
	}{
		{name: "unknown Type", change: func(policy *domain.MutationPolicy) { policy.TypeCode = "dynamic_plugin" }, want: ErrUnknownMutationPolicyType},
		{name: "unsafe target", change: func(policy *domain.MutationPolicy) { policy.CreateOperatorField = stringPointer("creator;drop") }, want: ErrInvalidMutationPolicyRules},
		{name: "duplicated target", change: func(policy *domain.MutationPolicy) { policy.ModifyOperatorField = stringPointer("creator") }, want: ErrInvalidMutationPolicyRules},
		{name: "primary-key target", change: func(policy *domain.MutationPolicy) { policy.CreateOperatorField = stringPointer("id") }, want: ErrInvalidMutationPolicyRules},
		{name: "Create target without ADD", change: func(policy *domain.MutationPolicy) { policy.AllowAdd = false }, want: ErrInvalidMutationPolicyRules},
		{name: "Modify target without ADD or MODIFY", change: func(policy *domain.MutationPolicy) {
			policy.AllowAdd = false
			policy.AllowModify = false
			policy.CreateOperatorField = nil
			policy.CreateTimeField = nil
		}, want: ErrInvalidMutationPolicyRules},
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

	readOnly := domain.MutationPolicy{TypeCode: SingleTableMutationPolicyType}
	if err := registry.Validate(readOnly); err != nil {
		t.Fatalf("read-only Mutation Policy should be valid: %v", err)
	}
}

func TestMutationPolicyDraftWithUnknownTypeCanBeCompletedBeforeActivation(t *testing.T) {
	catalog := &memoryMutationPolicyCatalog{policies: make(map[string]domain.MutationPolicy)}
	management := NewMutationPolicyManagement(catalog, NewMutationPolicyTypeRegistry(), "operator")
	candidate := validPutMutationPolicy("draft_mutation_v1")
	candidate.TypeCode = "unknown_type"
	if _, err := management.Create(context.Background(), candidate); err != nil {
		t.Fatalf("Draft should preserve an unknown Type for later completion: %v", err)
	}
	if _, err := management.Activate(context.Background(), candidate.Code); !errors.Is(err, ErrUnknownMutationPolicyType) {
		t.Fatalf("activation should fail closed on unknown Type, got %v", err)
	}
}

func TestMutationPolicyDraftRejectsValuesBlockedByCatalogConstraints(t *testing.T) {
	tests := []struct {
		name   string
		change func(*PutMutationPolicy)
	}{
		{name: "unsafe target", change: func(policy *PutMutationPolicy) { policy.CreateOperatorField = stringPointer("creator;drop") }},
		{name: "case-insensitive primary-key target", change: func(policy *PutMutationPolicy) { policy.CreateOperatorField = stringPointer("ID") }},
		{name: "duplicated target", change: func(policy *PutMutationPolicy) { policy.ModifyOperatorField = stringPointer("creator") }},
		{name: "case-insensitive duplicated target", change: func(policy *PutMutationPolicy) { policy.ModifyOperatorField = stringPointer("Creator") }},
		{name: "Create target without ADD", change: func(policy *PutMutationPolicy) { policy.AllowAdd = false }},
		{name: "Modify target without ADD or MODIFY", change: func(policy *PutMutationPolicy) {
			policy.AllowAdd = false
			policy.AllowModify = false
			policy.CreateOperatorField = nil
			policy.CreateTimeField = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := &memoryMutationPolicyCatalog{policies: make(map[string]domain.MutationPolicy)}
			management := NewMutationPolicyManagement(catalog, NewMutationPolicyTypeRegistry(), "operator")
			candidate := validPutMutationPolicy("constrained_mutation_v1")
			test.change(&candidate)
			if _, err := management.Create(context.Background(), candidate); !errors.Is(err, ErrInvalidMutationPolicyRules) {
				t.Fatalf("expected stable persistent-rule validation error, got %v", err)
			}
			if len(catalog.policies) != 0 {
				t.Fatalf("invalid Draft reached the Catalog: %#v", catalog.policies)
			}
		})
	}
}

func TestMutationPolicyTableValidationRejectsMissingGeneratedAndTypeIncompatibleTargets(t *testing.T) {
	registry := NewMutationPolicyTypeRegistry()
	base := mutationPolicyForValidation()
	schema := domain.TableSchema{
		Name: "managed_items", Compatible: true,
		Columns: []domain.Column{
			{Name: "id", Type: domain.ColumnTypeUInt64, AutoIncrement: true},
			{Name: "creator", Type: domain.ColumnTypeString},
			{Name: "created_at", Type: domain.ColumnTypeDateTime},
			{Name: "modifier", Type: domain.ColumnTypeString},
			{Name: "updated_at", Type: domain.ColumnTypeTimestamp},
			{Name: "generated_text", Type: domain.ColumnTypeString, Generated: true},
			{Name: "numeric_value", Type: domain.ColumnTypeInt64},
		},
	}
	if err := registry.ValidateForTable(base, schema); err != nil {
		t.Fatalf("valid Auto Fill targets were rejected: %v", err)
	}
	tests := []struct {
		name   string
		change func(*domain.MutationPolicy)
	}{
		{name: "missing", change: func(policy *domain.MutationPolicy) { policy.CreateOperatorField = stringPointer("missing") }},
		{name: "generated", change: func(policy *domain.MutationPolicy) { policy.CreateOperatorField = stringPointer("generated_text") }},
		{name: "operator requires text", change: func(policy *domain.MutationPolicy) { policy.CreateOperatorField = stringPointer("numeric_value") }},
		{name: "time requires temporal", change: func(policy *domain.MutationPolicy) { policy.CreateTimeField = stringPointer("creator") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := base
			test.change(&policy)
			if err := registry.ValidateForTable(policy, schema); !errors.Is(err, ErrInvalidMutationPolicyRules) {
				t.Fatalf("expected invalid Mutation Policy rules, got %v", err)
			}
		})
	}
}

func validPutMutationPolicy(code string) PutMutationPolicy {
	return PutMutationPolicy{
		Code: code, Name: "Standard", Description: "Reusable", TypeCode: SingleTableMutationPolicyType,
		AllowAdd: true, AllowModify: true, AllowDelete: false,
		CreateOperatorField: stringPointer("creator"), CreateTimeField: stringPointer("created_at"),
		ModifyOperatorField: stringPointer("modifier"), ModifyTimeField: stringPointer("updated_at"),
	}
}

func mutationPolicyForValidation() domain.MutationPolicy {
	candidate, _ := draftMutationPolicy(validPutMutationPolicy("standard_mutation_v1"))
	return candidate
}

func stringPointer(value string) *string { return &value }

type memoryMutationPolicyCatalog struct {
	policies map[string]domain.MutationPolicy
}

func (catalog *memoryMutationPolicyCatalog) CreateMutationPolicy(_ context.Context, policy domain.MutationPolicy, operator string) (domain.MutationPolicy, error) {
	if _, found := catalog.policies[policy.Code]; found {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyExists
	}
	now := time.Now().UTC()
	policy.Creator, policy.Modifier, policy.CreatedAt, policy.UpdatedAt = operator, operator, now, now
	catalog.policies[policy.Code] = policy
	return policy, nil
}

func (catalog *memoryMutationPolicyCatalog) ListMutationPolicies(context.Context) ([]domain.MutationPolicy, error) {
	result := make([]domain.MutationPolicy, 0, len(catalog.policies))
	for _, policy := range catalog.policies {
		result = append(result, policy)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result, nil
}

func (catalog *memoryMutationPolicyCatalog) GetMutationPolicy(_ context.Context, code string) (domain.MutationPolicy, error) {
	policy, found := catalog.policies[code]
	if !found {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyNotFound
	}
	return policy, nil
}

func (catalog *memoryMutationPolicyCatalog) ReplaceDraftMutationPolicy(_ context.Context, replacement domain.MutationPolicy, operator string) (domain.MutationPolicy, error) {
	current, found := catalog.policies[replacement.Code]
	if !found {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyNotFound
	}
	if current.Status != domain.PolicyStatusDraft {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyStateConflict
	}
	replacement.Creator, replacement.CreatedAt, replacement.Modifier, replacement.UpdatedAt = current.Creator, current.CreatedAt, operator, time.Now().UTC()
	catalog.policies[replacement.Code] = replacement
	return replacement, nil
}

func (catalog *memoryMutationPolicyCatalog) SetMutationPolicyStatus(_ context.Context, code string, from, to domain.PolicyStatus, operator string) (domain.MutationPolicy, error) {
	policy, found := catalog.policies[code]
	if !found {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyNotFound
	}
	if policy.Status != from {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyStateConflict
	}
	policy.Status, policy.Modifier, policy.UpdatedAt = to, operator, time.Now().UTC()
	catalog.policies[code] = policy
	return policy, nil
}

func (catalog *memoryMutationPolicyCatalog) UpdateMutationPolicyMetadata(_ context.Context, code, name, description, operator string) (domain.MutationPolicy, error) {
	policy, found := catalog.policies[code]
	if !found {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyNotFound
	}
	if policy.Status == domain.PolicyStatusDraft {
		return domain.MutationPolicy{}, domain.ErrMutationPolicyStateConflict
	}
	policy.Name, policy.Description, policy.Modifier, policy.UpdatedAt = name, description, operator, time.Now().UTC()
	catalog.policies[code] = policy
	return policy, nil
}

func (catalog *memoryMutationPolicyCatalog) DeleteDraftMutationPolicy(_ context.Context, code string) error {
	policy, found := catalog.policies[code]
	if !found {
		return domain.ErrMutationPolicyNotFound
	}
	if policy.Status != domain.PolicyStatusDraft {
		return domain.ErrMutationPolicyStateConflict
	}
	delete(catalog.policies, code)
	return nil
}
