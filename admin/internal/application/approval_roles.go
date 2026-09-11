package application

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type ApprovalRole = domain.ApprovalRole
type ApprovalRoleChange = domain.ApprovalRoleChange

var ErrApprovalRoleFields = domain.ErrApprovalRoleFields
var ErrApprovalRoleNotFound = domain.ErrApprovalRoleNotFound
var ErrApprovalRoleReferenced = domain.ErrApprovalRoleReferenced

var ErrApprovalRoleNotSaved = domain.ErrApprovalRoleNotSaved

var ErrApprovalRoleVersion = domain.ErrApprovalRoleVersion

type ApprovalRoleManagement struct{ repository domain.ApprovalRoleRepository }

func NewApprovalRoleManagement(repository domain.ApprovalRoleRepository) *ApprovalRoleManagement {
	return &ApprovalRoleManagement{repository: repository}
}

func (m *ApprovalRoleManagement) List(ctx context.Context, query, after string, limit int) ([]ApprovalRole, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 200 || limit < 1 || limit > 100 {
		return nil, ErrApprovalRoleFields
	}
	if after != "" && !validApprovalID(after) {
		return nil, ErrApprovalRoleFields
	}
	return m.repository.ListApprovalRoles(ctx, query, after, limit)
}
func (m *ApprovalRoleManagement) Get(ctx context.Context, id string) (ApprovalRole, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return ApprovalRole{}, err
	}
	if !validApprovalID(id) {
		return ApprovalRole{}, ErrApprovalRoleFields
	}
	return m.repository.GetApprovalRole(ctx, id)
}
func (m *ApprovalRoleManagement) Save(ctx context.Context, change ApprovalRoleChange) (ApprovalRole, error) {
	actor, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return ApprovalRole{}, err
	}
	change.ActorID = actor
	change.Name = strings.TrimSpace(change.Name)
	if change.Name == "" || utf8.RuneCountInString(change.Name) > 200 || utf8.RuneCountInString(change.Description) > 2000 || !utf8.ValidString(change.Description) || len(change.MemberIDs) > 1000 || !roleRequestKey.MatchString(change.RequestKey) {
		return ApprovalRole{}, ErrApprovalRoleFields
	}
	if change.ID != "" && (!validApprovalID(change.ID) || change.ExpectedVersion == 0) {
		return ApprovalRole{}, ErrApprovalRoleFields
	}
	seen := map[string]bool{}
	members := make([]string, 0, len(change.MemberIDs))
	for _, id := range change.MemberIDs {
		if !validApprovalID(id) {
			return ApprovalRole{}, ErrApprovalRoleFields
		}
		if !seen[id] {
			members = append(members, id)
			seen[id] = true
		}
	}
	sort.Strings(members)
	change.MemberIDs = members
	return m.repository.SaveApprovalRole(ctx, change)
}
func validApprovalID(id string) bool {
	_, err := domain.NormalizeAccountSelector(domain.AccountSelector{ID: id})
	return err == nil
}

func (m *ApprovalRoleManagement) Delete(ctx context.Context, id string, version uint64, key string) (ApprovalRole, error) {
	actor, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return ApprovalRole{}, err
	}
	if !validApprovalID(id) || version == 0 || !roleRequestKey.MatchString(key) {
		return ApprovalRole{}, ErrApprovalRoleFields
	}
	return m.repository.DeleteApprovalRole(ctx, ApprovalRoleChange{ActorID: actor, ID: id, ExpectedVersion: version, RequestKey: key})
}
