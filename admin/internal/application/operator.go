package application

import "context"

// AuthenticatedOperator is an immutable result of checking a Login Session.
// Its Account ID can only be populated by Authentication in this package.
// It remains valid for the already-authenticated request if that session is
// revoked while the request is executing.
type AuthenticatedOperator struct{ accountID string }
type operatorContextKey struct{}

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
		return AuthenticatedOperator{accountID: account.ID}, nil
	}
	current, err := a.Current(ctx, token)
	if err != nil {
		return AuthenticatedOperator{}, err
	}
	return AuthenticatedOperator{accountID: current.Account.ID}, nil
}

func requestOperator(ctx context.Context) (string, error) {
	operator, ok := ctx.Value(operatorContextKey{}).(AuthenticatedOperator)
	if !ok || operator.accountID == "" {
		return "", ErrSession
	}
	return operator.accountID, nil
}
