package mysql

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func (a *Adapter) ListRoleAccounts(ctx context.Context, query, after string, limit int) ([]domain.RoleAccount, error) {
	accounts := make([]domain.RoleAccount, 0)
	db := a.gorm.WithContext(ctx).Table(accountTable).Select("id,username,display_name,enabled,roles,role_version").Where("id > ?", after)
	if query != "" {
		pattern := containsPattern(query)
		db = db.Where("username LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!' OR id = ?", pattern, pattern, query)
	}
	err := db.Order("id").Limit(limit).Find(&accounts).Error
	return accounts, authError(err)
}

func (a *Adapter) ChangeAccountRoles(ctx context.Context, change domain.RoleChange) (domain.RoleAccount, error) {
	var account domain.RoleAccount
	digest := roleChangeDigest(change)
	err := a.authTransaction(ctx, time.Now(), func(tx *gorm.DB) error {
		var actor domain.LocalAccount
		if err := tx.Table(accountTable).Where("id = ?", change.ActorID).Take(&actor).Error; err != nil {
			return err
		}
		if !actor.Enabled || !actor.Roles.Allows(domain.RoleAdmin) {
			return domain.ErrPermissionDenied
		}
		var previous storedRoleEvent
		err := tx.Table(roleHistoryTable).Where("actor_id = ? AND request_key = ?", change.ActorID, change.RequestKey).Take(&previous).Error
		if err == nil {
			if previous.RequestDigest != digest {
				return domain.ErrRoleIdempotencyConflict
			}
			return json.Unmarshal(previous.Result, &account)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		err = tx.Table(accountTable).Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,username,display_name,enabled,roles,role_version").Where("id = ?", change.AccountID).Take(&account).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		if account.RoleVersion != change.ExpectedVersion {
			return domain.ErrRoleVersionConflict
		}
		if account.Enabled && account.Roles&domain.RoleAdmin != 0 && change.Roles&domain.RoleAdmin == 0 {
			if err := retainEnabledAdministrator(tx, account.ID); err != nil {
				return err
			}
		}
		if err := tx.Table(accountTable).Where("id = ?", account.ID).Updates(map[string]any{"roles": change.Roles, "role_version": gorm.Expr("role_version + 1")}).Error; err != nil {
			return err
		}
		before := account.Roles
		account.Roles = change.Roles
		account.RoleVersion++
		return saveRoleEvent(tx, "account", change.ActorID, before, account, change.RequestKey, digest, time.Now())
	})
	return account, err
}

// GrantAccountAdmin is available only to the database-authorized maintenance tool.
// It preserves other grants and is safe to repeat during bootstrap or recovery.
func (a *Adapter) GrantAccountAdmin(ctx context.Context, id string, now time.Time) error {
	return a.authTransaction(ctx, now, func(tx *gorm.DB) error {
		var account domain.LocalAccount
		err := tx.Table(accountTable).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&account).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		if !account.Enabled {
			return domain.ErrAccountDisabled
		}
		if account.Roles&domain.RoleAdmin != 0 {
			return nil
		}
		if err := tx.Table(accountTable).Where("id = ?", id).Updates(map[string]any{"roles": account.Roles | domain.RoleAdmin, "role_version": gorm.Expr("role_version + 1")}).Error; err != nil {
			return err
		}
		key := make([]byte, 16)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		result := domain.RoleAccount{ID: account.ID, Username: account.Username, DisplayName: account.DisplayName, Enabled: account.Enabled, Roles: account.Roles | domain.RoleAdmin, RoleVersion: account.RoleVersion + 1}
		return saveRoleEvent(tx, "maintenance", "", account.Roles, result, hex.EncodeToString(key), roleChangeDigest(domain.RoleChange{AccountID: id, Roles: result.Roles, ExpectedVersion: account.RoleVersion}), now)
	})
}

const roleHistoryTable = "rcc_account_role_history"

type storedRoleEvent struct {
	domain.RoleEvent `gorm:"embedded"`
	RequestKey       string
	RequestDigest    string
	Result           []byte
}

func roleChangeDigest(change domain.RoleChange) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%d", change.ActorID, change.AccountID, change.Roles, change.ExpectedVersion)))
	return hex.EncodeToString(sum[:])
}
func saveRoleEvent(tx *gorm.DB, kind, actor string, before domain.AccountRoles, result domain.RoleAccount, key, digest string, now time.Time) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return tx.Table(roleHistoryTable).Create(&storedRoleEvent{RoleEvent: domain.RoleEvent{ActorKind: kind, ActorID: actor, AccountID: result.ID, BeforeRoles: before, AfterRoles: result.Roles, Version: result.RoleVersion, CreatedAt: now.UTC().Truncate(time.Microsecond)}, RequestKey: key, RequestDigest: digest, Result: data}).Error
}
func (a *Adapter) RoleHistory(ctx context.Context, id string, before uint64, limit int) ([]domain.RoleEvent, error) {
	events := make([]domain.RoleEvent, 0)
	db := a.gorm.WithContext(ctx).Table(roleHistoryTable).Select("id,actor_kind,actor_id,account_id,before_roles,after_roles,version,created_at").Where("account_id = ?", id)
	if before != 0 {
		db = db.Where("id < ?", before)
	}
	err := db.Order("id DESC").Limit(limit).Find(&events).Error
	return events, authError(err)
}

// Every grant, demotion and account enable/disable holds rcc_auth_control_lock
// until commit, so two administrators cannot concurrently remove the last two.
func retainEnabledAdministrator(tx *gorm.DB, id string) error {
	var count int64
	if err := tx.Table(accountTable).Where("id <> ? AND enabled = TRUE AND roles & ? <> 0", id, domain.RoleAdmin).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrLastAdministrator
	}
	return nil
}
