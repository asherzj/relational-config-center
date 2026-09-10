package domain

import (
	"context"
	"encoding/json"
	"time"
)

// TableFieldPolicy guides Web interaction; it never grants execution authority.
// A nil DefaultValue means no prefill; JSON null means explicit SQL NULL.
type TableFieldPolicy struct {
	FieldName, DisplayName, Description                  string
	DisplayOrder                                         int
	IsVisible, IsQueryable                               bool
	QueryOperators                                       []QueryOperator
	UIType                                               string
	UIOptions                                            FieldUIOptions
	EditableOnAdd, EditableOnModify, IsRequired, Enabled bool
	DefaultValue                                         json.RawMessage
	Creator, Modifier                                    string
	CreatedAt, UpdatedAt                                 time.Time
}

type FieldUIOptions struct {
	Options []FieldOption `json:"options"`
	Min     *string       `json:"min,omitempty"`
	Max     *string       `json:"max,omitempty"`
	Step    *string       `json:"step,omitempty"`
}

type FieldOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type TableFieldPolicyCatalog interface {
	ReadFieldPolicies(context.Context, string) ([]TableFieldPolicy, error)
	ReplaceFieldPolicies(context.Context, string, []TableFieldPolicy, string) error
}
