package application

import (
	"context"
	"regexp"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrInvalidRoles        = domain.ErrInvalidRoles
	ErrRoleVersionConflict = domain.ErrRoleVersionConflict
)

var ErrLastAdministrator = domain.ErrLastAdministrator
var ErrRoleIdempotencyConflict = domain.ErrRoleIdempotencyConflict
var roleRequestKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{7,63}$`)

type RoleEvent = domain.RoleEvent

type RoleAccount = domain.RoleAccount

type AccountRoleManagement struct{ repository domain.AccountRoleRepository }

func NewAccountRoleManagement(accounts domain.AccountRoleRepository) *AccountRoleManagement {
	return &AccountRoleManagement{repository: accounts}
}

func requireRole(ctx context.Context, role AccountRoles) (string, error) {
	operator, ok := ctx.Value(operatorContextKey{}).(AuthenticatedOperator)
	if !ok || operator.accountID == "" {
		return "", ErrSession
	}
	if !operator.Allows(role) {
		return "", ErrPermissionDenied
	}
	return operator.accountID, nil
}

func (m *AccountRoleManagement) List(ctx context.Context, query, after string, limit int) ([]RoleAccount, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if len(query) > 64 || limit < 1 || limit > 100 {
		return nil, ErrAccountFields
	}
	if after != "" {
		if _, err := domain.NormalizeAccountSelector(domain.AccountSelector{ID: after}); err != nil {
			return nil, err
		}
	}
	return m.repository.ListRoleAccounts(ctx, query, after, limit)
}

func (m *AccountRoleManagement) Change(ctx context.Context, id string, names []string, version uint64, key string) (RoleAccount, error) {
	actor, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return RoleAccount{}, err
	}
	if _, err = domain.NormalizeAccountSelector(domain.AccountSelector{ID: id}); err != nil {
		return RoleAccount{}, err
	}
	roles, err := domain.ParseAccountRoles(names)
	if err != nil {
		return RoleAccount{}, err
	}
	if version == 0 || !roleRequestKey.MatchString(key) {
		return RoleAccount{}, ErrAccountFields
	}
	return m.repository.ChangeAccountRoles(ctx, domain.RoleChange{ActorID: actor, AccountID: id, Roles: roles, ExpectedVersion: version, RequestKey: key})
}

func (m *AccountRoleManagement) History(ctx context.Context, id string, before uint64, limit int) ([]RoleEvent, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return nil, err
	}
	if _, err := domain.NormalizeAccountSelector(domain.AccountSelector{ID: id}); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, ErrAccountFields
	}
	return m.repository.RoleHistory(ctx, id, before, limit)
}
