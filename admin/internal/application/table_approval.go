package application

import (
	"context"
	"sort"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type TableApprovalAssignment = domain.TableApprovalAssignment
type TableApprovalChange = domain.TableApprovalChange

var ErrTableApprovalVersion = domain.ErrTableApprovalVersion

func (m *ApprovalRoleManagement) Table(ctx context.Context, table string) (TableApprovalAssignment, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return TableApprovalAssignment{}, err
	}
	return m.repository.ReadTableApproval(ctx, table)
}
func (m *ApprovalRoleManagement) SaveTable(ctx context.Context, change TableApprovalChange) (TableApprovalAssignment, error) {
	actor, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return TableApprovalAssignment{}, err
	}
	change.ActorID = actor
	if strings.TrimSpace(change.TableName) == "" || len(change.TableName) > 256 || len(change.RoleIDs) > 100 || ValidateRecordVersion(change.ExpectedVersion) != nil || !roleRequestKey.MatchString(change.RequestKey) {
		return TableApprovalAssignment{}, ErrApprovalRoleFields
	}
	seen := map[string]bool{}
	for _, id := range change.RoleIDs {
		if !validApprovalID(id) || seen[id] {
			return TableApprovalAssignment{}, ErrApprovalRoleFields
		}
		seen[id] = true
	}
	sort.Strings(change.RoleIDs)
	return m.repository.SaveTableApproval(ctx, change)
}
