package application

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type stubPolicyRepository struct {
	policies  map[string]domain.Policy
	saveCalls int
}

func newStubPolicyRepository(policies ...domain.Policy) *stubPolicyRepository {
	repository := &stubPolicyRepository{policies: make(map[string]domain.Policy, len(policies))}
	for _, policy := range policies {
		repository.policies[policy.Resource] = policy
	}
	return repository
}

func (repository *stubPolicyRepository) Load(context.Context) ([]domain.Policy, error) {
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

func (repository *stubPolicyRepository) Save(_ context.Context, policy domain.Policy) error {
	repository.saveCalls++
	repository.policies[policy.Resource] = policy
	return nil
}

func (repository *stubPolicyRepository) Delete(_ context.Context, resource string) error {
	if _, exists := repository.policies[resource]; !exists {
		return domain.ErrPolicyNotFound
	}
	delete(repository.policies, resource)
	return nil
}

type stubVerifier struct {
	err error
}

func (verifier stubVerifier) VerifyPolicy(context.Context, domain.Policy) error {
	return verifier.err
}

func TestCatalogSaveVerifiesPersistsAndActivates(t *testing.T) {
	repository := newStubPolicyRepository(testPolicy())
	service := newCatalogService(t, repository, stubVerifier{})

	saved, err := service.Save(context.Background(), "gadgets", gadgetsPolicy())
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if repository.saveCalls != 1 {
		t.Fatalf("save calls = %d, want 1", repository.saveCalls)
	}
	if saved.DefaultPageSize != 20 || saved.MaxPageSize != 100 {
		t.Fatalf("saved document is not canonical: %+v", saved)
	}
	if _, ok := service.source.Current().Get("gadgets"); !ok {
		t.Fatal("saved policy is not active in the registry snapshot")
	}
	persisted, err := service.Get(context.Background(), "gadgets")
	if err != nil || persisted.Table != "gadgets_table" {
		t.Fatalf("Get() after save = %+v, %v", persisted, err)
	}
}

func TestCatalogSaveRejectsBeforePersisting(t *testing.T) {
	cases := []struct {
		name      string
		resource  string
		policy    domain.Policy
		verifier  stubVerifier
		wantSaved bool
	}{
		{
			name:     "resource mismatch",
			resource: "other",
			policy:   gadgetsPolicy(),
		},
		{
			name:     "invalid policy",
			resource: "gadgets",
			policy:   func() domain.Policy { policy := gadgetsPolicy(); policy.Table = "bad table!"; return policy }(),
		},
		{
			name:      "schema mismatch",
			resource:  "gadgets",
			policy:    gadgetsPolicy(),
			verifier:  stubVerifier{err: errors.New("physical table does not exist")},
			wantSaved: false,
		},
		{
			name:     "catalog self-management",
			resource: "gadgets",
			policy:   func() domain.Policy { policy := gadgetsPolicy(); policy.Table = domain.CatalogTable; return policy }(),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repository := newStubPolicyRepository(testPolicy())
			service := newCatalogService(t, repository, testCase.verifier)
			if _, err := service.Save(context.Background(), testCase.resource, testCase.policy); !domain.IsValidation(err) {
				t.Fatalf("Save() error = %v, want validation error", err)
			}
			if repository.saveCalls != 0 {
				t.Fatalf("rejected policy was persisted: %d save calls", repository.saveCalls)
			}
			if _, err := service.Get(context.Background(), "gadgets"); !errors.Is(err, domain.ErrPolicyNotFound) {
				t.Fatalf("rejected policy is visible: %v", err)
			}
		})
	}
}

func TestCatalogDeleteDeactivatesAndReportsMissing(t *testing.T) {
	repository := newStubPolicyRepository(testPolicy())
	service := newCatalogService(t, repository, stubVerifier{})

	if err := service.Delete(context.Background(), "widgets"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := service.source.Current().Get("widgets"); ok {
		t.Fatal("deleted policy is still active in the registry snapshot")
	}

	if err := service.Delete(context.Background(), "missing"); !errors.Is(err, domain.ErrPolicyNotFound) {
		t.Fatalf("Delete() unknown error = %v, want ErrPolicyNotFound", err)
	}
}

func TestCatalogListReturnsDocuments(t *testing.T) {
	service := newCatalogService(t, newStubPolicyRepository(testPolicy()), stubVerifier{})
	policies, err := service.List(context.Background())
	if err != nil || len(policies) != 1 || policies[0].Resource != "widgets" {
		t.Fatalf("List() = %+v, %v", policies, err)
	}
}

type catalogFixture struct {
	*CatalogService
}

func newCatalogService(t *testing.T, repository *stubPolicyRepository, verifier stubVerifier) *catalogFixture {
	t.Helper()
	registry, err := domain.NewRegistry(testPolicy())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	source := NewPolicySource(registry)
	return &catalogFixture{NewCatalogService(repository, verifier, source)}
}

func gadgetsPolicy() domain.Policy {
	return domain.Policy{
		Resource:    "gadgets",
		Table:       "gadgets_table",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		Fields: map[string]domain.FieldPolicy{
			"id": {
				Column:        "gadget_id",
				Type:          domain.TypeUnsigned,
				Readable:      true,
				Sortable:      true,
				AutoIncrement: true,
			},
			"name": {
				Column:           "gadget_name",
				Type:             domain.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        64,
			},
		},
	}
}
