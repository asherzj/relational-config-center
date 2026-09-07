package domain

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var ErrAccountNotFound = errors.New("account not found")

type AccountSelector struct{ ID, Username string }

var accountIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func NormalizeAccountSelector(selector AccountSelector) (AccountSelector, error) {
	selector.Username = NormalizeUsername(selector.Username)
	if (selector.ID == "") == (selector.Username == "") || (selector.ID != "" && !accountIDPattern.MatchString(selector.ID)) || (selector.Username != "" && !usernamePattern.MatchString(selector.Username)) {
		return AccountSelector{}, ErrAccountFields
	}
	return selector, nil
}

// AccountMaintenanceRepository is reachable only by database-authorized tools.
type AccountMaintenanceRepository interface {
	FindAccount(context.Context, AccountSelector) (LocalAccount, error)
	GrantAccountAdmin(context.Context, string, time.Time) error
	ResetAccountPassword(context.Context, string, string, time.Time) error
	SetAccountEnabled(context.Context, string, bool, time.Time) error
	CorrectAccountEmail(context.Context, string, string, time.Time) error
}
