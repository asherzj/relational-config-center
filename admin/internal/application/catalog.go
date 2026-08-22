package application

import (
	"context"
	"sync"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// CatalogService manages the Policy Catalog through dedicated use cases. The
// catalog never manages itself through the generic table API, and a saved
// policy is active immediately: the registry snapshot is rebuilt from the
// database after every write. Writes are serialized so two concurrent saves
// cannot install a stale snapshot; if a reload still fails after a committed
// write, the database remains the source of truth and the next write or
// restart heals the snapshot.
type CatalogService struct {
	writes   sync.Mutex
	policies domain.PolicyRepository
	verifier domain.SchemaVerifier
	source   *PolicySource
}

// NewCatalogService creates the policy-management use cases.
func NewCatalogService(policies domain.PolicyRepository, verifier domain.SchemaVerifier, source *PolicySource) *CatalogService {
	return &CatalogService{policies: policies, verifier: verifier, source: source}
}

// List returns every persisted policy document.
func (s *CatalogService) List(ctx context.Context) ([]domain.Policy, error) {
	return s.policies.Load(ctx)
}

// Get returns one persisted policy document.
func (s *CatalogService) Get(ctx context.Context, resource string) (domain.Policy, error) {
	policies, err := s.policies.Load(ctx)
	if err != nil {
		return domain.Policy{}, err
	}
	for _, policy := range policies {
		if policy.Resource == resource {
			return policy, nil
		}
	}
	return domain.Policy{}, domain.ErrPolicyNotFound
}

// Save validates, verifies against the live schema, persists, and activates
// one policy document in canonical form. Nothing is persisted when any
// check fails.
func (s *CatalogService) Save(ctx context.Context, resource string, policy domain.Policy) (domain.Policy, error) {
	if policy.Resource != resource {
		return domain.Policy{}, domain.Invalid("policy.resource", "must match the resource in the URL")
	}
	policy = policy.WithDefaults()
	if err := policy.Validate(); err != nil {
		return domain.Policy{}, domain.Invalid("policy", err.Error())
	}
	if err := s.verifier.VerifyPolicy(ctx, policy); err != nil {
		return domain.Policy{}, domain.Invalid("policy", err.Error())
	}
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := s.policies.Save(ctx, policy); err != nil {
		return domain.Policy{}, err
	}
	if err := s.reload(ctx); err != nil {
		return domain.Policy{}, err
	}
	return policy, nil
}

// Delete removes one policy document and deactivates the resource on the
// next request.
func (s *CatalogService) Delete(ctx context.Context, resource string) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := s.policies.Delete(ctx, resource); err != nil {
		return err
	}
	return s.reload(ctx)
}

func (s *CatalogService) reload(ctx context.Context) error {
	policies, err := s.policies.Load(ctx)
	if err != nil {
		return err
	}
	registry, err := domain.NewRegistry(policies...)
	if err != nil {
		return err
	}
	s.source.Replace(registry)
	return nil
}
