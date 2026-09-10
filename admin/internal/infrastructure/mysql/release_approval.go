package mysql

import (
	"context"
	"slices"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func readApprovalEnvironment(ctx context.Context, tx *gorm.DB, order domain.ReleaseOrder) (domain.ReleaseApprovalEnvironment, error) {
	result := domain.ReleaseApprovalEnvironment{Approvals: order.Approvals, Roles: []domain.ApprovalRole{}, Accounts: []domain.ApprovalAccount{}}
	if order.State == "DRAFT" {
		result.Approvals = []domain.ReleaseTableApproval{}
		for _, table := range order.TableNames {
			assignment, err := readTableApproval(tx.WithContext(ctx), table)
			if err != nil {
				return result, application.ErrReleaseUnavailable
			}
			result.Approvals = append(result.Approvals, domain.ReleaseTableApproval{TableName: table, Roles: assignment.Roles, State: "PENDING"})
		}
	} else if order.State == "CANCELLED" && len(result.Approvals) == 0 {
		result.Approvals = []domain.ReleaseTableApproval{}
	} else if len(result.Approvals) != len(order.TableNames) {
		return result, application.ErrReleaseUnavailable
	}
	ids := []string{}
	for _, approval := range result.Approvals {
		for _, role := range approval.Roles {
			if !slices.Contains(ids, role.ID) {
				ids = append(ids, role.ID)
			}
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		role, err := readApprovalRole(tx.WithContext(ctx), id)
		if err != nil {
			return result, application.ErrReleaseUnavailable
		}
		result.Roles = append(result.Roles, role)
	}
	// Current admins and members are read from one snapshot. Failed reads never
	// become an empty directory and therefore can never grant fallback authority.
	query := tx.WithContext(ctx).Table(accountTable).Select("id,enabled,roles,role_version,session_version").Order("id")
	members := []string{order.ApplicantID}
	for _, role := range result.Roles {
		for _, member := range role.Members {
			members = append(members, member.ID)
		}
	}
	if err := query.Where("id IN ? OR roles & ? <> 0", members, domain.RoleAdmin).Find(&result.Accounts).Error; err != nil {
		return result, application.ErrReleaseUnavailable
	}
	return result, nil
}
func (s *releaseOrderSession) ReadApprovalEnvironment(ctx context.Context, order domain.ReleaseOrder) (domain.ReleaseApprovalEnvironment, error) {
	if err := s.available(); err != nil {
		return domain.ReleaseApprovalEnvironment{}, err
	}
	return readApprovalEnvironment(ctx, s.database, order)
}
func (s *releaseOrderSession) ReferenceReleaseApprovalRoles(ctx context.Context, id string, approvals []domain.ReleaseTableApproval) error {
	if err := s.available(); err != nil {
		return err
	}
	for _, approval := range approvals {
		for _, role := range approval.Roles {
			if err := referenceApprovalRole(s.database.WithContext(ctx), role, "snapshot", id+":"+approval.TableName); err != nil {
				return application.ErrReleaseUnavailable
			}
		}
	}
	return nil
}
func (s *releaseOrderSession) CurrentReleaseAccount(ctx context.Context, id string) (domain.ApprovalAccount, error) {
	var account domain.ApprovalAccount
	if err := s.available(); err != nil {
		return account, err
	}
	if err := s.database.WithContext(ctx).Table(accountTable).Clauses(clause.Locking{Strength: "SHARE"}).Select("id,enabled,roles,role_version,session_version").Where("id=?", id).Take(&account).Error; err != nil {
		return account, application.ErrReleaseUnavailable
	}
	if !account.Enabled {
		return account, application.ErrPermissionDenied
	}
	return account, nil
}
