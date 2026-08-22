// Package managedtable defines policy-controlled access to relational tables.
package managedtable

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrUnknownResource     = errors.New("managed table not found")
	ErrRowNotFound         = errors.New("row not found")
	ErrConflict            = errors.New("row conflicts with existing data")
	ErrOperationNotAllowed = errors.New("operation is not allowed by the table policy")
)

// ValidationError identifies invalid client input without exposing SQL details.
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Invalid constructs an input validation error.
func Invalid(path, message string) error {
	return &ValidationError{Path: path, Message: message}
}

// IsValidation reports whether err represents invalid client input.
func IsValidation(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}

// ValueType controls validation and database/result conversion for a field.
type ValueType string

const (
	TypeString   ValueType = "string"
	TypeInteger  ValueType = "integer"
	TypeUnsigned ValueType = "unsigned_integer"
	TypeBoolean  ValueType = "boolean"
	TypeJSON     ValueType = "json"
	TypeTime     ValueType = "time"
)

// Operator is a filter operation understood by a table policy.
type Operator string

const (
	OperatorEqual              Operator = "eq"
	OperatorNotEqual           Operator = "ne"
	OperatorIn                 Operator = "in"
	OperatorContains           Operator = "contains"
	OperatorGreaterThan        Operator = "gt"
	OperatorGreaterThanOrEqual Operator = "gte"
	OperatorLessThan           Operator = "lt"
	OperatorLessThanOrEqual    Operator = "lte"
	OperatorIsNull             Operator = "is_null"
)

// Logic combines child filter expressions.
type Logic string

const (
	LogicAnd Logic = "and"
	LogicOr  Logic = "or"
)

// Direction controls sort order.
type Direction string

const (
	DirectionAscending  Direction = "asc"
	DirectionDescending Direction = "desc"
)

// Filter is either a leaf condition or an AND/OR group.
type Filter struct {
	Logic    Logic    `json:"logic,omitempty"`
	Items    []Filter `json:"items,omitempty"`
	Field    string   `json:"field,omitempty"`
	Operator Operator `json:"operator,omitempty"`
	Value    any      `json:"value,omitempty"`
}

// Sort identifies one public field and direction.
type Sort struct {
	Field     string    `json:"field"`
	Direction Direction `json:"direction"`
}

// Page uses one-based page numbers.
type Page struct {
	Number int `json:"number"`
	Size   int `json:"size"`
}

// QuerySpec is the database-independent query accepted from Admin clients.
type QuerySpec struct {
	Filter *Filter `json:"filter,omitempty"`
	Sort   []Sort  `json:"sort,omitempty"`
	Page   Page    `json:"page"`
}

// PageInfo describes the returned window and full filtered count.
type PageInfo struct {
	Number int   `json:"number"`
	Size   int   `json:"size"`
	Total  int64 `json:"total"`
}

// PageResult contains public field names only.
type PageResult struct {
	Rows []map[string]any `json:"rows"`
	Page PageInfo         `json:"page"`
}

// MutationResult describes a completed create, update, or delete.
type MutationResult struct {
	AffectedRows int64  `json:"affected_rows"`
	Key          string `json:"key,omitempty"`
}

// Repository persists policy-approved operations. Implementations may depend on
// a concrete database, while callers remain database-independent.
type Repository interface {
	Query(context.Context, Policy, QuerySpec) (PageResult, error)
	Create(context.Context, Policy, map[string]any) (MutationResult, error)
	Update(context.Context, Policy, any, map[string]any) (MutationResult, error)
	Delete(context.Context, Policy, any) (MutationResult, error)
}
