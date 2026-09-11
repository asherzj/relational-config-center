package application

import (
	"context"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// AuthenticatedOperator is an immutable result of checking a Login Session.
// Its Account ID can only be populated by Authentication in this package.
// It remains valid for the already-authenticated request if that session is
// revoked while the request is executing.
type AuthenticatedOperator struct {
	accountID string
	roles     domain.AccountRoles
}
type operatorContextKey struct{}

func (operator AuthenticatedOperator) AccountID() string { return operator.accountID }

type AccountRoles = domain.AccountRoles

const (
	RoleViewer    = domain.RoleViewer
	RoleEditor    = domain.RoleEditor
	RolePublisher = domain.RolePublisher
	RoleAdmin     = domain.RoleAdmin
)

var ErrPermissionDenied = domain.ErrPermissionDenied

func (operator AuthenticatedOperator) Allows(role AccountRoles) bool {
	return operator.roles.Allows(role)
}

func (operator AuthenticatedOperator) Bind(ctx context.Context) context.Context {
	return context.WithValue(ctx, operatorContextKey{}, operator)
}

// AuthenticateRequest checks current account/session state without extending
// activity. Changes additionally require the session's CSRF proof.
func (a *Authentication) AuthenticateRequest(ctx context.Context, token, csrf string, change bool) (AuthenticatedOperator, error) {
	if change {
		account, err := a.authorizeChange(ctx, token, csrf)
		if err != nil {
			return AuthenticatedOperator{}, err
		}
		return AuthenticatedOperator{accountID: account.ID, roles: account.Roles}, nil
	}
	current, err := a.Current(ctx, token)
	if err != nil {
		return AuthenticatedOperator{}, err
	}
	return AuthenticatedOperator{accountID: current.Account.ID, roles: current.Account.Roles}, nil
}
