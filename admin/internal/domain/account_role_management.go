package domain

import (
	"context"
	"time"
)

// RoleAccount exposes only the identity and grants needed by administrators.
type RoleAccount struct {
	ID          string
	Username    string
	DisplayName string
	Enabled     bool
	Roles       AccountRoles
	RoleVersion uint64
}

type RoleChange struct {
	ActorID         string
	AccountID       string
	Roles           AccountRoles
	ExpectedVersion uint64
	RequestKey      string
}

type AccountRoleRepository interface {
	ListRoleAccounts(context.Context, string, string, int) ([]RoleAccount, error)
	RoleHistory(context.Context, string, uint64, int) ([]RoleEvent, error)
	ChangeAccountRoles(context.Context, RoleChange) (RoleAccount, error)
}

// RoleEvent retains permanent identities, independently of display-name changes.
type RoleEvent struct {
	ID          uint64
	ActorKind   string
	ActorID     string
	AccountID   string
	BeforeRoles AccountRoles
	AfterRoles  AccountRoles
	Version     uint64
	CreatedAt   time.Time
}
