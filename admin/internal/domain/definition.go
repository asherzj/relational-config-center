package domain

import "sort"

// Definition is the frontend-safe view of a Table Policy.
type Definition struct {
	Resource   string               `json:"resource"`
	PrimaryKey string               `json:"primary_key"`
	Operations OperationDefinition  `json:"operations"`
	Fields     []FieldDefinition    `json:"fields"`
	Limits     QueryLimitDefinition `json:"limits"`
}

// OperationDefinition tells Web which mutation controls it may render.
type OperationDefinition struct {
	Create bool `json:"create"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

// FieldDefinition omits physical column names by design.
type FieldDefinition struct {
	Name             string     `json:"name"`
	Type             ValueType  `json:"type"`
	Nullable         bool       `json:"nullable"`
	Readable         bool       `json:"readable"`
	Creatable        bool       `json:"creatable"`
	Updatable        bool       `json:"updatable"`
	Sortable         bool       `json:"sortable"`
	RequiredOnCreate bool       `json:"required_on_create"`
	MinLength        int        `json:"min_length,omitempty"`
	MaxLength        int        `json:"max_length,omitempty"`
	AllowedValues    []string   `json:"allowed_values,omitempty"`
	FilterOperators  []Operator `json:"filter_operators,omitempty"`
}

// QueryLimitDefinition exposes limits that affect frontend query builders.
type QueryLimitDefinition struct {
	DefaultPageSize int `json:"default_page_size"`
	MaxPageSize     int `json:"max_page_size"`
	MaxFilterDepth  int `json:"max_filter_depth"`
	MaxFilterNodes  int `json:"max_filter_nodes"`
	MaxInValues     int `json:"max_in_values"`
}

// DefinitionOf projects a policy into its frontend-safe form.
func DefinitionOf(policy Policy) Definition {
	names := make([]string, 0, len(policy.Fields))
	for name := range policy.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]FieldDefinition, 0, len(names))
	for _, name := range names {
		field := policy.Fields[name]
		fields = append(fields, FieldDefinition{
			Name:             name,
			Type:             field.Type,
			Nullable:         field.Nullable,
			Readable:         field.Readable,
			Creatable:        field.Creatable,
			Updatable:        field.Updatable,
			Sortable:         field.Sortable,
			RequiredOnCreate: field.RequiredOnCreate,
			MinLength:        field.MinLength,
			MaxLength:        field.MaxLength,
			AllowedValues:    append([]string(nil), field.AllowedValues...),
			FilterOperators:  append([]Operator(nil), field.FilterOperators...),
		})
	}
	return Definition{
		Resource:   policy.Resource,
		PrimaryKey: policy.PrimaryKey,
		Operations: OperationDefinition{
			Create: policy.AllowCreate,
			Update: policy.AllowUpdate,
			Delete: policy.AllowDelete,
		},
		Fields: fields,
		Limits: QueryLimitDefinition{
			DefaultPageSize: policy.DefaultPageSize,
			MaxPageSize:     policy.MaxPageSize,
			MaxFilterDepth:  policy.MaxFilterDepth,
			MaxFilterNodes:  policy.MaxFilterNodes,
			MaxInValues:     policy.MaxInValues,
		},
	}
}
