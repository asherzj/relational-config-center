package managedtable

import (
	"fmt"
	"regexp"
	"sort"
)

const (
	defaultPageSize       = 20
	defaultMaxPageSize    = 100
	defaultMaxFilterDepth = 4
	defaultMaxFilterNodes = 32
	defaultMaxInValues    = 100
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// FieldPolicy maps a public field to a trusted physical column and its allowed operations.
type FieldPolicy struct {
	Column           string
	Type             ValueType
	Nullable         bool
	Readable         bool
	Creatable        bool
	Updatable        bool
	Sortable         bool
	RequiredOnCreate bool
	AutoIncrement    bool
	MinLength        int
	MaxLength        int
	AllowedValues    []string
	FilterOperators  []Operator
}

func (f FieldPolicy) permits(operator Operator) bool {
	for _, allowed := range f.FilterOperators {
		if operator == allowed {
			return true
		}
	}
	return false
}

// Policy defines the complete client-visible surface of one managed table.
type Policy struct {
	Resource        string
	Table           string
	PrimaryKey      string
	Fields          map[string]FieldPolicy
	AllowCreate     bool
	AllowUpdate     bool
	AllowDelete     bool
	DefaultSort     []Sort
	DefaultPageSize int
	MaxPageSize     int
	MaxFilterDepth  int
	MaxFilterNodes  int
	MaxInValues     int
}

// WithDefaults returns a copy with safe query limits filled in.
func (p Policy) WithDefaults() Policy {
	if p.DefaultPageSize == 0 {
		p.DefaultPageSize = defaultPageSize
	}
	if p.MaxPageSize == 0 {
		p.MaxPageSize = defaultMaxPageSize
	}
	if p.MaxFilterDepth == 0 {
		p.MaxFilterDepth = defaultMaxFilterDepth
	}
	if p.MaxFilterNodes == 0 {
		p.MaxFilterNodes = defaultMaxFilterNodes
	}
	if p.MaxInValues == 0 {
		p.MaxInValues = defaultMaxInValues
	}
	if len(p.DefaultSort) == 0 && p.PrimaryKey != "" {
		p.DefaultSort = []Sort{{Field: p.PrimaryKey, Direction: DirectionAscending}}
	}
	return p
}

// Validate rejects invalid or unsafe policies at process startup.
func (p Policy) Validate() error {
	p = p.WithDefaults()
	if p.Resource == "" {
		return fmt.Errorf("resource is required")
	}
	if !identifierPattern.MatchString(p.Resource) {
		return fmt.Errorf("policy resource %q must use lowercase letters, digits, and underscores", p.Resource)
	}
	if !identifierPattern.MatchString(p.Table) {
		return fmt.Errorf("policy %q: physical table %q is not a safe identifier", p.Resource, p.Table)
	}
	if p.PrimaryKey == "" {
		return fmt.Errorf("policy %q: primary key is required", p.Resource)
	}
	if p.DefaultPageSize < 1 || p.MaxPageSize < p.DefaultPageSize {
		return fmt.Errorf("policy %q: invalid page limits", p.Resource)
	}
	if p.MaxFilterDepth < 1 || p.MaxFilterNodes < 1 || p.MaxInValues < 1 {
		return fmt.Errorf("policy %q: invalid query complexity limits", p.Resource)
	}
	primary, ok := p.Fields[p.PrimaryKey]
	if !ok {
		return fmt.Errorf("policy %q: primary key field %q is not declared", p.Resource, p.PrimaryKey)
	}
	if primary.Column == "" || !primary.Readable || !primary.Sortable {
		return fmt.Errorf("policy %q: primary key must map to a readable and sortable column", p.Resource)
	}
	if !primary.AutoIncrement && (!primary.Creatable || !primary.RequiredOnCreate) {
		return fmt.Errorf("policy %q: a non-auto-increment primary key must be required and creatable", p.Resource)
	}
	columns := make(map[string]string, len(p.Fields))
	for publicName, field := range p.Fields {
		if !identifierPattern.MatchString(publicName) {
			return fmt.Errorf("policy %q: public field %q is not a safe identifier", p.Resource, publicName)
		}
		if !identifierPattern.MatchString(field.Column) {
			return fmt.Errorf("policy %q field %q: physical column %q is not a safe identifier", p.Resource, publicName, field.Column)
		}
		if previous, exists := columns[field.Column]; exists {
			return fmt.Errorf("policy %q: fields %q and %q map to the same column", p.Resource, previous, publicName)
		}
		columns[field.Column] = publicName
		if !validValueType(field.Type) {
			return fmt.Errorf("policy %q field %q: unsupported type %q", p.Resource, publicName, field.Type)
		}
		if field.RequiredOnCreate && !field.Creatable {
			return fmt.Errorf("policy %q field %q: required-on-create fields must be creatable", p.Resource, publicName)
		}
		if field.MinLength < 0 || field.MaxLength < 0 || (field.MaxLength > 0 && field.MinLength > field.MaxLength) {
			return fmt.Errorf("policy %q field %q: invalid string length constraints", p.Resource, publicName)
		}
		if (field.MinLength > 0 || field.MaxLength > 0 || len(field.AllowedValues) > 0) && field.Type != TypeString {
			return fmt.Errorf("policy %q field %q: string constraints require a string field", p.Resource, publicName)
		}
		if (field.Sortable || len(field.FilterOperators) > 0) && !field.Readable {
			return fmt.Errorf("policy %q field %q: sortable and filterable fields must be readable", p.Resource, publicName)
		}
		for _, operator := range field.FilterOperators {
			if !validOperator(operator) {
				return fmt.Errorf("policy %q field %q: unsupported operator %q", p.Resource, publicName, operator)
			}
			if operator == OperatorContains && field.Type != TypeString {
				return fmt.Errorf("policy %q field %q: contains requires a string field", p.Resource, publicName)
			}
		}
	}
	for _, item := range p.DefaultSort {
		field, exists := p.Fields[item.Field]
		if !exists || !field.Sortable {
			return fmt.Errorf("policy %q: default sort field %q is not sortable", p.Resource, item.Field)
		}
		if item.Direction != DirectionAscending && item.Direction != DirectionDescending {
			return fmt.Errorf("policy %q: invalid default sort direction %q", p.Resource, item.Direction)
		}
	}
	return nil
}

func validValueType(value ValueType) bool {
	switch value {
	case TypeString, TypeInteger, TypeUnsigned, TypeBoolean, TypeJSON, TypeTime:
		return true
	default:
		return false
	}
}

func validOperator(operator Operator) bool {
	switch operator {
	case OperatorEqual, OperatorNotEqual, OperatorIn, OperatorContains,
		OperatorGreaterThan, OperatorGreaterThanOrEqual,
		OperatorLessThan, OperatorLessThanOrEqual, OperatorIsNull:
		return true
	default:
		return false
	}
}

// Registry contains immutable policies indexed by their public resource name.
type Registry struct {
	policies map[string]Policy
	names    []string
}

// NewRegistry validates policies and rejects duplicate public resources.
func NewRegistry(policies ...Policy) (*Registry, error) {
	registry := &Registry{policies: make(map[string]Policy, len(policies))}
	for _, candidate := range policies {
		policy := candidate.WithDefaults()
		if err := policy.Validate(); err != nil {
			return nil, err
		}
		if _, exists := registry.policies[policy.Resource]; exists {
			return nil, fmt.Errorf("duplicate managed table resource %q", policy.Resource)
		}
		registry.policies[policy.Resource] = policy
		registry.names = append(registry.names, policy.Resource)
	}
	sort.Strings(registry.names)
	return registry, nil
}

// Get resolves a public resource without accepting a physical table name.
func (r *Registry) Get(resource string) (Policy, bool) {
	policy, ok := r.policies[resource]
	return policy, ok
}

// Names returns registered resources in stable order.
func (r *Registry) Names() []string {
	return append([]string(nil), r.names...)
}
