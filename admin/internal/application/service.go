// Package application orchestrates managed-table use cases for transport layers.
package application

import (
	"context"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Service validates table policies and query specifications before persistence.
type Service struct {
	registry   *domain.Registry
	repository domain.Repository
}

// NewService creates the application service used by HTTP handlers.
func NewService(registry *domain.Registry, repository domain.Repository) *Service {
	return &Service{registry: registry, repository: repository}
}

// List returns frontend-safe definitions for every registered managed table.
func (s *Service) List() []domain.Definition {
	names := s.registry.Names()
	definitions := make([]domain.Definition, 0, len(names))
	for _, name := range names {
		policy, _ := s.registry.Get(name)
		definitions = append(definitions, domain.DefinitionOf(policy))
	}
	return definitions
}

// Describe returns the frontend-safe policy for one resource.
func (s *Service) Describe(resource string) (domain.Definition, error) {
	policy, ok := s.registry.Get(resource)
	if !ok {
		return domain.Definition{}, domain.ErrUnknownResource
	}
	return domain.DefinitionOf(policy), nil
}

// Query validates and normalizes a client query before database compilation.
func (s *Service) Query(ctx context.Context, resource string, query domain.QuerySpec) (domain.PageResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return domain.PageResult{}, err
	}
	prepared, err := policy.PrepareQuery(query)
	if err != nil {
		return domain.PageResult{}, err
	}
	return s.repository.Query(ctx, policy, prepared)
}

// Create inserts one row using creatable public fields only.
func (s *Service) Create(ctx context.Context, resource string, values map[string]any) (domain.MutationResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return domain.MutationResult{}, err
	}
	if !policy.AllowCreate {
		return domain.MutationResult{}, domain.ErrOperationNotAllowed
	}
	prepared, err := policy.PrepareMutation(values, true)
	if err != nil {
		return domain.MutationResult{}, err
	}
	return s.repository.Create(ctx, policy, prepared)
}

// Update changes one row selected by the policy's primary key.
func (s *Service) Update(ctx context.Context, resource, rawKey string, values map[string]any) (domain.MutationResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return domain.MutationResult{}, err
	}
	if !policy.AllowUpdate {
		return domain.MutationResult{}, domain.ErrOperationNotAllowed
	}
	key, err := policy.PrepareKey(rawKey)
	if err != nil {
		return domain.MutationResult{}, err
	}
	prepared, err := policy.PrepareMutation(values, false)
	if err != nil {
		return domain.MutationResult{}, err
	}
	return s.repository.Update(ctx, policy, key, prepared)
}

// Delete removes one row selected by the policy's primary key.
func (s *Service) Delete(ctx context.Context, resource, rawKey string) (domain.MutationResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return domain.MutationResult{}, err
	}
	if !policy.AllowDelete {
		return domain.MutationResult{}, domain.ErrOperationNotAllowed
	}
	key, err := policy.PrepareKey(rawKey)
	if err != nil {
		return domain.MutationResult{}, err
	}
	return s.repository.Delete(ctx, policy, key)
}

func (s *Service) policy(resource string) (domain.Policy, error) {
	policy, ok := s.registry.Get(resource)
	if !ok {
		return domain.Policy{}, domain.ErrUnknownResource
	}
	return policy, nil
}
