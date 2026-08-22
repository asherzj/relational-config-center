package domain

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

// CatalogTable is the physical table backing the Policy Catalog. A Table
// Policy can never target it, so the catalog is not reachable through the
// generic table API.
const CatalogTable = "table_policies"

// FieldPolicy maps a public field to a trusted physical column and its allowed operations.
// The JSON tags define the persisted policy-document format used by the Policy
// Catalog; the generic table API never exposes these fields directly.
type FieldPolicy struct {
	Column           string     `json:"column"`
	Type             ValueType  `json:"type"`
	Nullable         bool       `json:"nullable,omitempty"`
	Readable         bool       `json:"readable,omitempty"`
	Creatable        bool       `json:"creatable,omitempty"`
	Updatable        bool       `json:"updatable,omitempty"`
	Sortable         bool       `json:"sortable,omitempty"`
	RequiredOnCreate bool       `json:"required_on_create,omitempty"`
	AutoIncrement    bool       `json:"auto_increment,omitempty"`
	MinLength        int        `json:"min_length,omitempty"`
	MaxLength        int        `json:"max_length,omitempty"`
	AllowedValues    []string   `json:"allowed_values,omitempty"`
	FilterOperators  []Operator `json:"filter_operators,omitempty"`
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
	Resource        string                 `json:"resource"`
	Table           string                 `json:"table"`
	PrimaryKey      string                 `json:"primary_key"`
	Fields          map[string]FieldPolicy `json:"fields"`
	AllowCreate     bool                   `json:"allow_create,omitempty"`
	AllowUpdate     bool                   `json:"allow_update,omitempty"`
	AllowDelete     bool                   `json:"allow_delete,omitempty"`
	DefaultSort     []Sort                 `json:"default_sort,omitempty"`
	DefaultPageSize int                    `json:"default_page_size,omitempty"`
	MaxPageSize     int                    `json:"max_page_size,omitempty"`
	MaxFilterDepth  int                    `json:"max_filter_depth,omitempty"`
	MaxFilterNodes  int                    `json:"max_filter_nodes,omitempty"`
	MaxInValues     int                    `json:"max_in_values,omitempty"`
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
	if p.Table == CatalogTable {
		return fmt.Errorf("policy %q: the Policy Catalog table %q cannot manage itself", p.Resource, CatalogTable)
	}
	if p.Resource == CatalogTable {
		return fmt.Errorf("policy resource %q is reserved for the Policy Catalog", p.Resource)
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
