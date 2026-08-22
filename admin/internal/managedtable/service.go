package managedtable

import (
	"context"
	"fmt"
)

// Service validates table policies and query specifications before persistence.
type Service struct {
	registry   *Registry
	repository Repository
}

// NewService creates the application service used by HTTP handlers.
func NewService(registry *Registry, repository Repository) *Service {
	return &Service{registry: registry, repository: repository}
}

// List returns frontend-safe definitions for every registered managed table.
func (s *Service) List() []Definition {
	names := s.registry.Names()
	definitions := make([]Definition, 0, len(names))
	for _, name := range names {
		policy, _ := s.registry.Get(name)
		definitions = append(definitions, definitionOf(policy))
	}
	return definitions
}

// Describe returns the frontend-safe policy for one resource.
func (s *Service) Describe(resource string) (Definition, error) {
	policy, ok := s.registry.Get(resource)
	if !ok {
		return Definition{}, ErrUnknownResource
	}
	return definitionOf(policy), nil
}

// Query validates and normalizes a client query before database compilation.
func (s *Service) Query(ctx context.Context, resource string, query QuerySpec) (PageResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return PageResult{}, err
	}
	prepared, err := prepareQuery(policy, query)
	if err != nil {
		return PageResult{}, err
	}
	return s.repository.Query(ctx, policy, prepared)
}

// Create inserts one row using creatable public fields only.
func (s *Service) Create(ctx context.Context, resource string, values map[string]any) (MutationResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return MutationResult{}, err
	}
	if !policy.AllowCreate {
		return MutationResult{}, ErrOperationNotAllowed
	}
	prepared, err := prepareMutation(policy, values, true)
	if err != nil {
		return MutationResult{}, err
	}
	return s.repository.Create(ctx, policy, prepared)
}

// Update changes one row selected by the policy's primary key.
func (s *Service) Update(ctx context.Context, resource, rawKey string, values map[string]any) (MutationResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return MutationResult{}, err
	}
	if !policy.AllowUpdate {
		return MutationResult{}, ErrOperationNotAllowed
	}
	key, err := prepareKey(policy, rawKey)
	if err != nil {
		return MutationResult{}, err
	}
	prepared, err := prepareMutation(policy, values, false)
	if err != nil {
		return MutationResult{}, err
	}
	return s.repository.Update(ctx, policy, key, prepared)
}

// Delete removes one row selected by the policy's primary key.
func (s *Service) Delete(ctx context.Context, resource, rawKey string) (MutationResult, error) {
	policy, err := s.policy(resource)
	if err != nil {
		return MutationResult{}, err
	}
	if !policy.AllowDelete {
		return MutationResult{}, ErrOperationNotAllowed
	}
	key, err := prepareKey(policy, rawKey)
	if err != nil {
		return MutationResult{}, err
	}
	return s.repository.Delete(ctx, policy, key)
}

func (s *Service) policy(resource string) (Policy, error) {
	policy, ok := s.registry.Get(resource)
	if !ok {
		return Policy{}, ErrUnknownResource
	}
	return policy, nil
}

func prepareQuery(policy Policy, query QuerySpec) (QuerySpec, error) {
	if query.Page.Number == 0 {
		query.Page.Number = 1
	}
	if query.Page.Size == 0 {
		query.Page.Size = policy.DefaultPageSize
	}
	if query.Page.Number < 1 {
		return QuerySpec{}, Invalid("page.number", "must be at least 1")
	}
	if query.Page.Size < 1 || query.Page.Size > policy.MaxPageSize {
		return QuerySpec{}, Invalid("page.size", fmt.Sprintf("must be between 1 and %d", policy.MaxPageSize))
	}
	maxInt := int(^uint(0) >> 1)
	if query.Page.Number-1 > maxInt/query.Page.Size {
		return QuerySpec{}, Invalid("page.number", "is too large")
	}

	if len(query.Sort) == 0 {
		query.Sort = append([]Sort(nil), policy.DefaultSort...)
	}
	seenSorts := make(map[string]struct{}, len(query.Sort))
	for index, sortItem := range query.Sort {
		path := fmt.Sprintf("sort[%d]", index)
		field, ok := policy.Fields[sortItem.Field]
		if !ok || !field.Sortable {
			return QuerySpec{}, Invalid(path+".field", "field is not sortable")
		}
		if _, exists := seenSorts[sortItem.Field]; exists {
			return QuerySpec{}, Invalid(path+".field", "field is sorted more than once")
		}
		seenSorts[sortItem.Field] = struct{}{}
		if sortItem.Direction == "" {
			query.Sort[index].Direction = DirectionAscending
		} else if sortItem.Direction != DirectionAscending && sortItem.Direction != DirectionDescending {
			return QuerySpec{}, Invalid(path+".direction", "must be asc or desc")
		}
	}
	if _, exists := seenSorts[policy.PrimaryKey]; !exists {
		query.Sort = append(query.Sort, Sort{Field: policy.PrimaryKey, Direction: DirectionAscending})
	}

	if query.Filter != nil {
		nodes := 0
		prepared, err := prepareFilter(policy, *query.Filter, "filter", 1, &nodes)
		if err != nil {
			return QuerySpec{}, err
		}
		query.Filter = &prepared
	}
	return query, nil
}

func prepareFilter(policy Policy, filter Filter, path string, depth int, nodes *int) (Filter, error) {
	(*nodes)++
	if *nodes > policy.MaxFilterNodes {
		return Filter{}, Invalid(path, fmt.Sprintf("query contains more than %d filter nodes", policy.MaxFilterNodes))
	}
	if depth > policy.MaxFilterDepth {
		return Filter{}, Invalid(path, fmt.Sprintf("query exceeds maximum filter depth %d", policy.MaxFilterDepth))
	}

	isGroup := filter.Logic != "" || len(filter.Items) > 0
	if isGroup {
		if filter.Logic != LogicAnd && filter.Logic != LogicOr {
			return Filter{}, Invalid(path+".logic", `must be "and" or "or"`)
		}
		if len(filter.Items) == 0 {
			return Filter{}, Invalid(path+".items", "must contain at least one filter")
		}
		if filter.Field != "" || filter.Operator != "" || filter.Value != nil {
			return Filter{}, Invalid(path, "a filter group cannot also be a field condition")
		}
		for index, child := range filter.Items {
			prepared, err := prepareFilter(policy, child, fmt.Sprintf("%s.items[%d]", path, index), depth+1, nodes)
			if err != nil {
				return Filter{}, err
			}
			filter.Items[index] = prepared
		}
		return filter, nil
	}

	if filter.Field == "" {
		return Filter{}, Invalid(path+".field", "is required")
	}
	field, ok := policy.Fields[filter.Field]
	if !ok || !field.Readable {
		return Filter{}, Invalid(path+".field", "field is not queryable")
	}
	if !field.permits(filter.Operator) {
		return Filter{}, Invalid(path+".operator", "operator is not permitted for this field")
	}

	if filter.Operator == OperatorIsNull {
		if filter.Value == nil {
			filter.Value = true
		} else if _, ok := filter.Value.(bool); !ok {
			return Filter{}, Invalid(path+".value", "must be a boolean when provided")
		}
		return filter, nil
	}
	if filter.Operator == OperatorIn {
		items, ok := filter.Value.([]any)
		if !ok || len(items) == 0 {
			return Filter{}, Invalid(path+".value", "must be a non-empty array")
		}
		if len(items) > policy.MaxInValues {
			return Filter{}, Invalid(path+".value", fmt.Sprintf("must contain at most %d values", policy.MaxInValues))
		}
		for index, item := range items {
			normalized, err := normalizeValue(fmt.Sprintf("%s.value[%d]", path, index), field, item)
			if err != nil {
				return Filter{}, err
			}
			items[index] = normalized
		}
		filter.Value = items
		return filter, nil
	}

	normalized, err := normalizeValue(path+".value", field, filter.Value)
	if err != nil {
		return Filter{}, err
	}
	filter.Value = normalized
	return filter, nil
}

func prepareMutation(policy Policy, values map[string]any, creating bool) (map[string]any, error) {
	if len(values) == 0 {
		return nil, Invalid("values", "must contain at least one field")
	}
	prepared := make(map[string]any, len(values))
	for publicName, value := range values {
		field, ok := policy.Fields[publicName]
		if !ok {
			return nil, Invalid("values."+publicName, "field is not declared by the table policy")
		}
		allowed := field.Updatable
		if creating {
			allowed = field.Creatable
		}
		if !allowed {
			return nil, Invalid("values."+publicName, "field cannot be changed by this operation")
		}
		normalized, err := normalizeValue("values."+publicName, field, value)
		if err != nil {
			return nil, err
		}
		prepared[publicName] = normalized
	}
	if creating {
		for publicName, field := range policy.Fields {
			if field.RequiredOnCreate {
				if _, exists := prepared[publicName]; !exists {
					return nil, Invalid("values."+publicName, "is required when creating a row")
				}
			}
		}
	}
	return prepared, nil
}

func prepareKey(policy Policy, raw string) (any, error) {
	field := policy.Fields[policy.PrimaryKey]
	return normalizePathValue("key", raw, field)
}
