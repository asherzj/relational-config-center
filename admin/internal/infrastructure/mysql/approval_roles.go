package mysql

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

const approvalRoleTable = "rcc_approval_roles"
const approvalMemberTable = "rcc_approval_role_members"
const approvalRequestTable = "rcc_approval_role_requests"
const approvalReferenceTable = "rcc_approval_role_references"

// Persistence fields stay in the adapter; relationship and response fields are
// populated explicitly after reading this row.
type storedApprovalRole struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	Version     uint64
	Creator     string
	Modifier    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (row storedApprovalRole) domain() domain.ApprovalRole {
	return domain.ApprovalRole{ID: row.ID, Name: row.Name, Description: row.Description, Enabled: row.Enabled, Version: row.Version, Creator: row.Creator, Modifier: row.Modifier, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

type approvalRoleRequest struct {
	ActorID       string
	RequestKey    string
	RequestDigest string
	Result        []byte
	CreatedAt     time.Time
}

func (a *Adapter) ListApprovalRoles(ctx context.Context, query, after string, limit int) ([]domain.ApprovalRole, error) {
	roles := make([]domain.ApprovalRole, 0)
	err := a.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		db := tx.Table(approvalRoleTable).Where("id > ?", after)
		if query != "" {
			pattern := containsPattern(query)
			db = db.Where("name LIKE ? ESCAPE '!' OR id = ?", pattern, query)
		}
		var rows []storedApprovalRole
		if err := db.Order("id").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			roles = append(roles, row.domain())
		}
		for i := range roles {
			if err := loadApprovalMembers(tx, &roles[i]); err != nil {
				return err
			}
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return roles, authError(err)
}
func (a *Adapter) GetApprovalRole(ctx context.Context, id string) (domain.ApprovalRole, error) {
	var role domain.ApprovalRole
	err := a.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; role, err = readApprovalRole(tx, id); return err }, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return role, authError(err)
}
func readApprovalRole(tx *gorm.DB, id string) (domain.ApprovalRole, error) {
	var row storedApprovalRole
	err := tx.Table(approvalRoleTable).Where("id = ?", id).Take(&row).Error
	role := row.domain()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return role, domain.ErrApprovalRoleNotFound
	}
	if err != nil {
		return role, err
	}
	return role, loadApprovalMembers(tx, &role)
}
func loadApprovalMembers(tx *gorm.DB, role *domain.ApprovalRole) error {
	role.Members = make([]domain.ApprovalMember, 0)
	if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM "+approvalReferenceTable+" WHERE role_id=?)", role.ID).Scan(&role.Referenced).Error; err != nil {
		return err
	}
	return tx.Table(approvalMemberTable+" AS m").Select("a.id,a.username,a.display_name,a.enabled").Joins("JOIN "+accountTable+" AS a ON a.id=m.account_id").Where("m.role_id = ?", role.ID).Order("a.id").Find(&role.Members).Error
}
func (a *Adapter) SaveApprovalRole(ctx context.Context, change domain.ApprovalRoleChange) (domain.ApprovalRole, error) {
	return a.approvalRoleTransaction(ctx, "save", change, func(tx *gorm.DB) (domain.ApprovalRole, error) {
		var role domain.ApprovalRole
		if len(change.MemberIDs) > 0 {
			var count int64
			if err := tx.Table(accountTable).Where("id IN ?", change.MemberIDs).Count(&count).Error; err != nil {
				return role, err
			}
			if count != int64(len(change.MemberIDs)) {
				return role, domain.ErrAccountNotFound
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if change.ID == "" {
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				return role, err
			}
			raw[6] = (raw[6] & 15) | 64
			raw[8] = (raw[8] & 63) | 128
			row := storedApprovalRole{ID: fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:]), Name: change.Name, Description: change.Description, Enabled: change.Enabled, Version: 1, Creator: change.ActorID, Modifier: change.ActorID, CreatedAt: now, UpdatedAt: now}
			if err := tx.Table(approvalRoleTable).Create(&row).Error; err != nil {
				return role, err
			}
			role = row.domain()
		} else {
			var err error
			role, err = readApprovalRole(tx, change.ID)
			if err != nil {
				return role, err
			}
			if role.Version != change.ExpectedVersion {
				return role, domain.ErrApprovalRoleVersion
			}
			if err := tx.Table(approvalRoleTable).Where("id = ?", role.ID).Updates(map[string]any{"name": change.Name, "description": change.Description, "enabled": change.Enabled, "version": role.Version + 1, "modifier": change.ActorID, "updated_at": now}).Error; err != nil {
				return role, err
			}
		}
		if err := tx.Exec("DELETE FROM "+approvalMemberTable+" WHERE role_id=?", role.ID).Error; err != nil {
			return role, err
		}
		for _, id := range change.MemberIDs {
			if err := tx.Exec("INSERT INTO "+approvalMemberTable+"(role_id,account_id) VALUES (?,?)", role.ID, id).Error; err != nil {
				return role, err
			}
		}
		role, err := readApprovalRole(tx, role.ID)
		if err != nil {
			return role, err
		}
		return role, nil
	})
}

func approvalRoleDigest(operation string, change domain.ApprovalRoleChange) (string, error) {
	data, err := json.Marshal(struct {
		Operation string
		Change    domain.ApprovalRoleChange
	}{operation, change})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
func readApprovalRoleRequest(tx *gorm.DB, actor, key, digest string, role *domain.ApprovalRole) (bool, error) {
	var previous approvalRoleRequest
	err := tx.Table(approvalRequestTable).Where("actor_id=? AND request_key=?", actor, key).Take(&previous).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if previous.RequestDigest != digest {
		return true, domain.ErrRoleIdempotencyConflict
	}
	return true, json.Unmarshal(previous.Result, role)
}
func saveApprovalRoleRequest(tx *gorm.DB, actor, key, digest string, role domain.ApprovalRole) error {
	data, err := json.Marshal(role)
	if err != nil {
		return err
	}
	return tx.Table(approvalRequestTable).Create(&approvalRoleRequest{ActorID: actor, RequestKey: key, RequestDigest: digest, Result: data, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}).Error
}

// References are permanent facts, even after a table is unassigned. The foreign
// key and shared authorization lock serialize reference writers with deletion.
func (a *Adapter) DeleteApprovalRole(ctx context.Context, change domain.ApprovalRoleChange) (domain.ApprovalRole, error) {
	return a.approvalRoleTransaction(ctx, "delete", change, func(tx *gorm.DB) (domain.ApprovalRole, error) {
		role, err := readApprovalRole(tx, change.ID)
		if err != nil {
			return role, err
		}
		if role.Version != change.ExpectedVersion {
			return role, domain.ErrApprovalRoleVersion
		}
		if role.Referenced {
			return role, domain.ErrApprovalRoleReferenced
		}
		if err := tx.Exec("DELETE FROM "+approvalRoleTable+" WHERE id=?", role.ID).Error; err != nil {
			return role, err
		}
		role.Deleted = true
		return role, nil
	})
}

// Every role write shares account-maintenance serialization, current actor
// authorization and the durable original-result check before changing data.
func (a *Adapter) approvalRoleTransaction(ctx context.Context, operation string, change domain.ApprovalRoleChange, mutate func(*gorm.DB) (domain.ApprovalRole, error)) (domain.ApprovalRole, error) {
	var role domain.ApprovalRole
	commitAttempted := false
	err := a.authTransaction(ctx, time.Now(), func(tx *gorm.DB) error {
		var actor domain.LocalAccount
		if err := tx.Table(accountTable).Where("id=?", change.ActorID).Take(&actor).Error; err != nil {
			return err
		}
		if !actor.Enabled || !actor.Roles.Allows(domain.RoleAdmin) {
			return domain.ErrPermissionDenied
		}
		digest, err := approvalRoleDigest(operation, change)
		if err != nil {
			return err
		}
		found, err := readApprovalRoleRequest(tx, change.ActorID, change.RequestKey, digest, &role)
		if err != nil {
			return err
		}
		if !found {
			role, err = mutate(tx)
			if err != nil {
				return err
			}
			if err := saveApprovalRoleRequest(tx, change.ActorID, change.RequestKey, digest, role); err != nil {
				return err
			}
		}
		// Returning nil is the only path on which GORM attempts COMMIT. Failures
		// before this point cannot commit this attempt, even if rollback loses its connection.
		commitAttempted = true
		return nil
	})
	if !commitAttempted && (errors.Is(err, domain.ErrAuthUnavailable) || errors.Is(err, domain.ErrAuthTimeout)) {
		return role, domain.ErrApprovalRoleNotSaved
	}
	return role, err
}
