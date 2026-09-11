package mysql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

func readTableApproval(tx *gorm.DB, table string) (domain.TableApprovalAssignment, error) {
	result := domain.TableApprovalAssignment{TableName: table, Version: "0", RoleIDs: []string{}, Roles: []domain.ApprovalRoleIdentity{}}
	var row struct {
		Version uint64
		RoleIDs []byte
	}
	err := tx.Table("rcc_table_approval_assignments").Where("table_name=?", table).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.Version = strconv.FormatUint(row.Version, 10)
	if err = json.Unmarshal(row.RoleIDs, &result.RoleIDs); err != nil {
		return result, err
	}
	for _, id := range result.RoleIDs {
		role, err := readApprovalRole(tx, id)
		if err != nil {
			return result, err
		}
		result.Roles = append(result.Roles, domain.ApprovalRoleIdentity{ID: id, Name: role.Name})
	}
	return result, nil
}
func requireApprovalTable(tx *gorm.DB, table string) error {
	var count int64
	if err := tx.Table("rcc_table_policies").Where("BINARY table_name = ?", table).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrApprovalRoleFields
	}
	return nil
}
func (a *Adapter) ReadTableApproval(ctx context.Context, table string) (domain.TableApprovalAssignment, error) {
	var result domain.TableApprovalAssignment
	err := a.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireApprovalTable(tx, table); err != nil {
			return err
		}
		var err error
		result, err = readTableApproval(tx, table)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, authError(err)
}
func (a *Adapter) SaveTableApproval(ctx context.Context, change domain.TableApprovalChange) (domain.TableApprovalAssignment, error) {
	var result domain.TableApprovalAssignment
	commitAttempted := false
	err := a.authTransaction(ctx, time.Now(), func(tx *gorm.DB) error {
		var actor domain.LocalAccount
		if err := tx.Table(accountTable).Where("id=?", change.ActorID).Take(&actor).Error; err != nil {
			return err
		}
		if !actor.Enabled || !actor.Roles.Allows(domain.RoleAdmin) {
			return domain.ErrPermissionDenied
		}
		encoded, err := json.Marshal(change)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(encoded)
		var previous struct{ Digest, Result []byte }
		err = tx.Table("rcc_table_approval_requests").Where("actor_id=? AND request_key=?", change.ActorID, change.RequestKey).Take(&previous).Error
		if err == nil {
			if !bytes.Equal(previous.Digest, digest[:]) {
				return domain.ErrRoleIdempotencyConflict
			}
			if err = json.Unmarshal(previous.Result, &result); err != nil {
				return err
			}
			commitAttempted = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := requireApprovalTable(tx, change.TableName); err != nil {
			return err
		}
		current, err := readTableApproval(tx, change.TableName)
		if err != nil {
			return err
		}
		if current.Version != change.ExpectedVersion {
			return domain.ErrTableApprovalVersion
		}
		version, err := strconv.ParseUint(current.Version, 10, 64)
		if err != nil || version == ^uint64(0) {
			return domain.ErrTableApprovalVersion
		}
		for _, id := range change.RoleIDs {
			role, err := readApprovalRole(tx, id)
			if err != nil {
				return err
			}
			if err := referenceApprovalRole(tx, domain.ApprovalRoleIdentity{ID: id, Name: role.Name}, "table", change.TableName); err != nil {
				return err
			}
		}
		ids, err := json.Marshal(change.RoleIDs)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO rcc_table_approval_assignments(table_name,version,role_ids) VALUES(?,?,?) ON DUPLICATE KEY UPDATE version=VALUES(version),role_ids=VALUES(role_ids)`, change.TableName, version+1, ids).Error; err != nil {
			return err
		}
		result, err = readTableApproval(tx, change.TableName)
		if err != nil {
			return err
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO rcc_table_approval_requests(actor_id,request_key,digest,result) VALUES(?,?,?,?)`, change.ActorID, change.RequestKey, digest[:], data).Error; err != nil {
			return err
		}
		commitAttempted = true
		return nil
	})
	if !commitAttempted && (errors.Is(err, domain.ErrAuthUnavailable) || errors.Is(err, domain.ErrAuthTimeout)) {
		return result, domain.ErrApprovalRoleNotSaved
	}
	return result, err
}
func referenceApprovalRole(tx *gorm.DB, role domain.ApprovalRoleIdentity, kind, key string) error {
	return tx.Exec(`INSERT INTO rcc_approval_role_references(role_id,reference_kind,reference_key,role_name,created_at) VALUES(?,?,?,?,UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE role_id=role_id`, role.ID, kind, key, role.Name).Error
}
