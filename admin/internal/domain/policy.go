package domain

import (
	"context"
	"errors"
)

var (
	ErrTablePolicyExists   = errors.New("table Policy already exists")
	ErrTablePolicyNotFound = errors.New("table Policy not found")
)

// JSONConfig is an object-valued, strategy-specific Policy configuration.
type JSONConfig []byte

// TablePolicy assigns one Query Policy and one Mutation Policy to a table.
// Catalog storage identity and audit columns are deliberately not part of the
// Aggregate's external representation.
type TablePolicy struct {
	TableName            string
	QueryPolicy          string
	QueryPolicyConfig    JSONConfig
	MutationPolicy       string
	MutationPolicyConfig JSONConfig
	Enabled              bool
}

// TablePolicyCatalog persists the current Policy Aggregate for each table.
type TablePolicyCatalog interface {
	Create(context.Context, TablePolicy, string) error
	List(context.Context) ([]TablePolicy, error)
	Get(context.Context, string) (TablePolicy, error)
	Replace(context.Context, TablePolicy, string) (TablePolicy, error)
	SetEnabled(context.Context, string, bool, string) (TablePolicy, error)
}
