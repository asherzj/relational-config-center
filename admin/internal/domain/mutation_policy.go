package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrMutationPolicyExists        = errors.New("Mutation Policy already exists")
	ErrMutationPolicyNotFound      = errors.New("Mutation Policy not found")
	ErrMutationPolicyStateConflict = errors.New("Mutation Policy state conflict")
)

// MutationPolicy is one reusable, versioned single-table mutation rule set.
// The four Auto Fill values are nullable target-column slots; their sources are
// fixed by the slot and cannot be supplied as Catalog data.
type MutationPolicy struct {
	Code                string
	Name                string
	Description         string
	TypeCode            string
	AllowAdd            bool
	AllowModify         bool
	AllowDelete         bool
	CreateOperatorField *string
	CreateTimeField     *string
	ModifyOperatorField *string
	ModifyTimeField     *string
	Status              PolicyStatus
	Creator             string
	Modifier            string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// MutationPolicyCatalog persists Mutation Policy definitions and applies
// lifecycle preconditions atomically.
type MutationPolicyCatalog interface {
	CreateMutationPolicy(context.Context, MutationPolicy, string) (MutationPolicy, error)
	ListMutationPolicies(context.Context) ([]MutationPolicy, error)
	GetMutationPolicy(context.Context, string) (MutationPolicy, error)
	ReplaceDraftMutationPolicy(context.Context, MutationPolicy, string) (MutationPolicy, error)
	SetMutationPolicyStatus(context.Context, string, PolicyStatus, PolicyStatus, string) (MutationPolicy, error)
	UpdateMutationPolicyMetadata(context.Context, string, string, string, string) (MutationPolicy, error)
	DeleteDraftMutationPolicy(context.Context, string) error
}
