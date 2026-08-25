package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrQueryPolicyExists        = errors.New("Query Policy already exists")
	ErrQueryPolicyNotFound      = errors.New("Query Policy not found")
	ErrQueryPolicyStateConflict = errors.New("Query Policy state conflict")
)

type PolicyStatus string

const (
	PolicyStatusDraft      PolicyStatus = "DRAFT"
	PolicyStatusActive     PolicyStatus = "ACTIVE"
	PolicyStatusDeprecated PolicyStatus = "DEPRECATED"
)

// QueryPolicy is one reusable, versioned query rule set. Code is its public
// identity; the numeric Catalog key is deliberately not part of the model.
type QueryPolicy struct {
	Code                  string
	Name                  string
	Description           string
	TypeCode              string
	DefaultOrderField     string
	DefaultOrderDirection string
	DefaultPageSize       int
	MaxPageSize           int
	Status                PolicyStatus
	Creator               string
	Modifier              string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// QueryPolicyCatalog persists Query Policy definitions and enforces lifecycle
// preconditions atomically so concurrent administrators cannot bypass them.
type QueryPolicyCatalog interface {
	CreateQueryPolicy(context.Context, QueryPolicy, string) (QueryPolicy, error)
	ListQueryPolicies(context.Context) ([]QueryPolicy, error)
	GetQueryPolicy(context.Context, string) (QueryPolicy, error)
	ReplaceDraftQueryPolicy(context.Context, QueryPolicy, string) (QueryPolicy, error)
	SetQueryPolicyStatus(context.Context, string, PolicyStatus, PolicyStatus, string) (QueryPolicy, error)
	UpdateQueryPolicyMetadata(context.Context, string, string, string, string) (QueryPolicy, error)
	DeleteDraftQueryPolicy(context.Context, string) error
}
