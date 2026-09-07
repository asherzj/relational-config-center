package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ domain.AccountMaintenanceRepository = (*Adapter)(nil)

func (a *Adapter) FindAccount(ctx context.Context, selector domain.AccountSelector) (domain.LocalAccount, error) {
	var account domain.LocalAccount
	query := a.gorm.WithContext(ctx).Table(accountTable)
	if selector.ID != "" {
		query = query.Where("id = ?", selector.ID)
	} else {
		query = query.Where("username = ?", selector.Username)
	}
	err := query.Take(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return account, domain.ErrAccountNotFound
	}
	return account, authError(err)
}

func (a *Adapter) ResetAccountPassword(ctx context.Context, id, hash string, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		result := tx.Table(accountTable).Where("id = ?", id).Updates(map[string]any{
			"password_hash":    hash,
			"password_version": gorm.Expr("password_version + 1"),
			"session_version":  gorm.Expr("session_version + 1"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrAccountNotFound
		}
		// The UPDATE holds the account row lock. Preserve the disabled-account
		// signal even when a maintainer resets its password before clients return.
		var account struct{ Enabled bool }
		if err := tx.Table(accountTable).Select("enabled").Where("id = ?", id).Take(&account).Error; err != nil {
			return err
		}
		if !account.Enabled {
			return nil
		}
		return tx.Table(sessionTable).Where("account_id = ?", id).Delete(&domain.LoginSession{}).Error
	})
}

func (a *Adapter) SetAccountEnabled(ctx context.Context, id string, enabled bool, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		var current domain.LocalAccount
		err := tx.Table(accountTable).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		if current.Enabled == enabled {
			return nil
		}
		if !enabled && current.Roles&domain.RoleAdmin != 0 {
			if err := retainEnabledAdministrator(tx, id); err != nil {
				return err
			}
		}
		// Keep identifiable old sessions until normal expiry cleanup so disabled
		// clients can destroy drafts. Advancing the version prevents revival.
		return tx.Table(accountTable).Where("id = ?", id).Updates(map[string]any{
			"enabled":         enabled,
			"session_version": gorm.Expr("session_version + 1"),
		}).Error
	})
}

func (a *Adapter) CorrectAccountEmail(ctx context.Context, id, email string, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		result := tx.Table(accountTable).Where("id = ?", id).Update("email", email)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrAccountNotFound
		}
		return nil
	})
}
