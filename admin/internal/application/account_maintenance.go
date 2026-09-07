package application

import (
	"context"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type AccountSelector = domain.AccountSelector

type LocalAccountSummary struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Enabled  bool   `json:"enabled"`
}

var ErrAccountNotFound = domain.ErrAccountNotFound

type AccountMaintenance struct {
	accounts  domain.AccountMaintenanceRepository
	passwords PasswordHasher
}

func NewAccountMaintenance(accounts domain.AccountMaintenanceRepository, passwords PasswordHasher) *AccountMaintenance {
	return &AccountMaintenance{accounts: accounts, passwords: passwords}
}

func (m *AccountMaintenance) Find(ctx context.Context, selector AccountSelector) (LocalAccountSummary, error) {
	selector, err := domain.NormalizeAccountSelector(selector)
	if err != nil {
		return LocalAccountSummary{}, err
	}
	account, err := m.accounts.FindAccount(ctx, selector)
	return LocalAccountSummary{ID: account.ID, Username: account.Username, Enabled: account.Enabled}, err
}

func (m *AccountMaintenance) ResetPassword(ctx context.Context, selector AccountSelector, password string) (LocalAccountSummary, error) {
	if !domain.ValidPassword(password) {
		return LocalAccountSummary{}, domain.ErrAccountFields
	}
	account, err := m.Find(ctx, selector)
	if err != nil {
		return LocalAccountSummary{}, err
	}
	hash, err := m.passwords.Hash(ctx, password)
	if err != nil {
		return LocalAccountSummary{}, err
	}
	err = m.accounts.ResetAccountPassword(ctx, account.ID, hash, time.Now())
	return account, err
}

func (m *AccountMaintenance) SetEnabled(ctx context.Context, selector AccountSelector, enabled bool) (LocalAccountSummary, error) {
	account, err := m.Find(ctx, selector)
	if err != nil {
		return LocalAccountSummary{}, err
	}
	err = m.accounts.SetAccountEnabled(ctx, account.ID, enabled, time.Now())
	account.Enabled = enabled
	return account, err
}

func (m *AccountMaintenance) CorrectEmail(ctx context.Context, selector AccountSelector, email string) (LocalAccountSummary, error) {
	email = domain.NormalizeEmail(email)
	if !domain.ValidEmail(email) {
		return LocalAccountSummary{}, domain.ErrAccountFields
	}
	account, err := m.Find(ctx, selector)
	if err != nil {
		return LocalAccountSummary{}, err
	}
	err = m.accounts.CorrectAccountEmail(ctx, account.ID, email, time.Now())
	return account, err
}
