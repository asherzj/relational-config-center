package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTablePolicyExists   = errors.New("table Policy already exists")
	ErrTablePolicyNotFound = errors.New("table Policy not found")
)

// TablePolicy assigns one Query Policy and one Mutation Policy to a table.
// Storage identity stays internal; audit metadata is part of the management
// projection exposed with the accepted created_at/updated_at names.
type TablePolicy struct {
	QueryPolicyCode    string
	MutationPolicyCode string
	TableName          string
	Enabled            bool
	Creator            string
	Modifier           string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// TablePolicyCatalog persists the current Policy Aggregate for each table.
type TablePolicyCatalog interface {
	Create(context.Context, TablePolicy, string) error
	List(context.Context) ([]TablePolicy, error)
	Get(context.Context, string) (TablePolicy, error)
	Replace(context.Context, TablePolicy, string) (TablePolicy, error)
	SetEnabled(context.Context, string, bool, string) (TablePolicy, error)
}
