package domain

import "fmt"

// PrepareQuery normalizes pagination, sorting, and filters against the policy
// and returns a specification that is safe to compile.
func (p Policy) PrepareQuery(query QuerySpec) (QuerySpec, error) {
	if query.Page.Number == 0 {
		query.Page.Number = 1
	}
	if query.Page.Size == 0 {
		query.Page.Size = p.DefaultPageSize
	}
	if query.Page.Number < 1 {
		return QuerySpec{}, Invalid("page.number", "must be at least 1")
	}
	if query.Page.Size < 1 || query.Page.Size > p.MaxPageSize {
		return QuerySpec{}, Invalid("page.size", fmt.Sprintf("must be between 1 and %d", p.MaxPageSize))
	}
	maxInt := int(^uint(0) >> 1)
	if query.Page.Number-1 > maxInt/query.Page.Size {
		return QuerySpec{}, Invalid("page.number", "is too large")
	}

	if len(query.Sort) == 0 {
		query.Sort = append([]Sort(nil), p.DefaultSort...)
	}
	seenSorts := make(map[string]struct{}, len(query.Sort))
	for index, sortItem := range query.Sort {
		path := fmt.Sprintf("sort[%d]", index)
		field, ok := p.Fields[sortItem.Field]
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
	if _, exists := seenSorts[p.PrimaryKey]; !exists {
		query.Sort = append(query.Sort, Sort{Field: p.PrimaryKey, Direction: DirectionAscending})
	}

	if query.Filter != nil {
		nodes := 0
		prepared, err := p.prepareFilter(*query.Filter, "filter", 1, &nodes)
		if err != nil {
			return QuerySpec{}, err
		}
		query.Filter = &prepared
	}
	return query, nil
}

func (p Policy) prepareFilter(filter Filter, path string, depth int, nodes *int) (Filter, error) {
	(*nodes)++
	if *nodes > p.MaxFilterNodes {
		return Filter{}, Invalid(path, fmt.Sprintf("query contains more than %d filter nodes", p.MaxFilterNodes))
	}
	if depth > p.MaxFilterDepth {
		return Filter{}, Invalid(path, fmt.Sprintf("query exceeds maximum filter depth %d", p.MaxFilterDepth))
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
			prepared, err := p.prepareFilter(child, fmt.Sprintf("%s.items[%d]", path, index), depth+1, nodes)
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
	field, ok := p.Fields[filter.Field]
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
		if len(items) > p.MaxInValues {
			return Filter{}, Invalid(path+".value", fmt.Sprintf("must contain at most %d values", p.MaxInValues))
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

// PrepareMutation validates field permissions, required fields, and value
// shapes, returning values keyed by public field name.
func (p Policy) PrepareMutation(values map[string]any, creating bool) (map[string]any, error) {
	if len(values) == 0 {
		return nil, Invalid("values", "must contain at least one field")
	}
	prepared := make(map[string]any, len(values))
	for publicName, value := range values {
		field, ok := p.Fields[publicName]
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
		for publicName, field := range p.Fields {
			if field.RequiredOnCreate {
				if _, exists := prepared[publicName]; !exists {
					return nil, Invalid("values."+publicName, "is required when creating a row")
				}
			}
		}
	}
	return prepared, nil
}

// PrepareKey parses a primary key from its path-parameter representation.
func (p Policy) PrepareKey(raw string) (any, error) {
	field := p.Fields[p.PrimaryKey]
	return normalizePathValue("key", raw, field)
}
