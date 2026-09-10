package application

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type ReleaseTableApproval = domain.ReleaseTableApproval

var ErrReleaseApprovalConflict = errors.New("approval qualification or confirmed scope changed")
var ErrReleaseApproverUnavailable = errors.New("an independent approver is required")

type ReleaseApproverUnavailable struct{ Tables []string }

func (e *ReleaseApproverUnavailable) Error() string {
	return fmt.Sprintf("independent approver required for %v", e.Tables)
}
func (e *ReleaseApproverUnavailable) Unwrap() error { return ErrReleaseApproverUnavailable }

type releaseApprovalReader interface {
	ReadApprovalEnvironment(context.Context, domain.ReleaseOrder) (domain.ReleaseApprovalEnvironment, error)
}

func approvalContext(environment domain.ReleaseApprovalEnvironment, order domain.ReleaseOrder, actor string) (domain.ReleaseApprovalContext, []domain.ReleaseApprovalSource) {
	result := domain.ReleaseApprovalContext{Tables: []domain.ReleaseApprovalTableStatus{}, ApprovableTables: []string{}}
	sources := []domain.ReleaseApprovalSource{}
	accounts := map[string]domain.ApprovalAccount{}
	for _, account := range environment.Accounts {
		accounts[account.ID] = account
	}
	roles := map[string]domain.ApprovalRole{}
	for _, role := range environment.Roles {
		roles[role.ID] = role
	}
	for _, approval := range environment.Approvals {
		status := domain.ReleaseApprovalTableStatus{TableName: approval.TableName, Mode: "COMPLETED", Reason: "该表已处理"}
		source := domain.ReleaseApprovalSource{TableName: approval.TableName, Roles: []domain.ApprovalRoleIdentity{}}
		if approval.State == "PENDING" {
			independent := false
			for _, identity := range approval.Roles {
				role := roles[identity.ID]
				if !role.Enabled {
					continue
				}
				for _, member := range role.Members {
					account := accounts[member.ID]
					if !account.Enabled || account.ID == order.ApplicantID {
						continue
					}
					independent = true
					if member.ID == actor {
						source.Roles = append(source.Roles, domain.ApprovalRoleIdentity{ID: role.ID, Name: role.Name})
					}
				}
			}
			if independent {
				status.Mode = "ROLE"
				status.Reason = "由提交时审批角色的当前合格成员处理"
				source.Source = "ROLE"
				status.CanApprove = len(source.Roles) > 0
			} else {
				status.Mode = "UNAVAILABLE"
				status.Reason = "缺少独立审批人，请补充启用的角色成员或非申请人 ADMIN"
				for _, account := range environment.Accounts {
					if account.Enabled && account.ID != order.ApplicantID && account.Roles.Allows(RoleAdmin) {
						status.Mode = "ADMIN"
					}
				}
				if status.Mode == "ADMIN" {
					status.Reason = "提交时角色没有合格独立成员，由非申请人 ADMIN 默认审批"
				}
				source.Source = "ADMIN"
				status.CanApprove = accounts[actor].Enabled && actor != order.ApplicantID && accounts[actor].Roles.Allows(RoleAdmin)
			}
			status.CanApprove = status.CanApprove && order.State == "PENDING_APPROVAL"
		}
		result.Tables = append(result.Tables, status)
		if status.CanApprove {
			result.ApprovableTables = append(result.ApprovableTables, approval.TableName)
			sources = append(sources, source)
		}
	}
	// Include the frozen responsibility, current role membership/status and account
	// qualification versions. A role change cannot silently reuse an old confirmation.
	digest := releaseDigest(struct {
		OrderID, Version, Actor string
		Environment             domain.ReleaseApprovalEnvironment
	}{order.ID, order.Version, actor, environment})
	result.Revision = hex.EncodeToString(digest)
	return result, sources
}
func (r *ReleaseOrders) reviewApprovals(ctx context.Context, order *domain.ReleaseOrder) error {
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return err
	}
	environment, err := r.store.ReadApprovalEnvironment(ctx, *order)
	if err != nil {
		return err
	}
	order.Approvals = environment.Approvals
	order.ApprovalContext, _ = approvalContext(environment, *order, actor)
	return nil
}
func freezeReleaseApprovals(ctx context.Context, s ReleaseOrderSession, order *domain.ReleaseOrder) error {
	environment, err := s.ReadApprovalEnvironment(ctx, *order)
	if err != nil {
		return err
	}
	view, _ := approvalContext(environment, *order, order.ApplicantID)
	missing := []string{}
	for _, table := range view.Tables {
		if table.Mode == "UNAVAILABLE" {
			missing = append(missing, table.TableName)
		}
	}
	if len(missing) > 0 {
		return &ReleaseApproverUnavailable{Tables: missing}
	}
	order.Approvals = environment.Approvals
	return s.ReferenceReleaseApprovalRoles(ctx, order.ID, order.Approvals)
}
func approvalScopeEqual(left, right []string) bool {
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func authorizeApprovalRequest(ctx context.Context, s ReleaseOrderSession, order *domain.ReleaseOrder, previous *domain.ReleaseOrder, input ReleaseDecisionInput) error {
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return err
	}
	if actor == order.ApplicantID {
		return ErrPermissionDenied
	}
	environment, err := s.ReadApprovalEnvironment(ctx, *order)
	if err != nil {
		return err
	}
	if previous != nil {
		// Replaying a committed request is not another decision on a completed table.
		// Recheck today's qualification for the original scope without reopening it.
		replay := *order
		replay.State = "PENDING_APPROVAL"
		environment.Approvals = slices.Clone(environment.Approvals)
		for i := range environment.Approvals {
			if slices.Contains(input.ConfirmedTables, environment.Approvals[i].TableName) {
				environment.Approvals[i].State = "PENDING"
			}
		}
		eligibility, _ := approvalContext(environment, replay, actor)
		for _, table := range input.ConfirmedTables {
			if !slices.Contains(eligibility.ApprovableTables, table) {
				return ErrPermissionDenied
			}
		}
		if len(input.ConfirmedTables) == 0 {
			return ErrPermissionDenied
		}
		return nil
	}
	order.ApprovalContext, _ = approvalContext(environment, *order, actor)
	if input.ExpectedApprovalRevision == "" || input.ConfirmedTables == nil {
		return ErrReleaseInvalid
	}
	if order.Version != input.ExpectedVersion {
		return ErrReleaseVersionConflict
	}
	if input.ExpectedApprovalRevision != order.ApprovalContext.Revision || !approvalScopeEqual(input.ConfirmedTables, order.ApprovalContext.ApprovableTables) {
		return ErrReleaseApprovalConflict
	}
	if len(order.ApprovalContext.ApprovableTables) == 0 {
		return ErrPermissionDenied
	}
	return nil
}
func applyReleaseDecision(ctx context.Context, s ReleaseOrderSession, order *domain.ReleaseOrder, action string, input ReleaseDecisionInput) error {
	actor, err := requireRole(ctx, RoleViewer)
	if err != nil {
		return err
	}
	environment, err := s.ReadApprovalEnvironment(ctx, *order)
	if err != nil {
		return err
	}
	_, sources := approvalContext(environment, *order, actor)
	now, err := s.DatabaseTime(ctx)
	if err != nil {
		return err
	}
	allApproved := true
	for i := range order.Approvals {
		approval := &order.Approvals[i]
		for _, source := range sources {
			if source.TableName == approval.TableName {
				approval.State = "APPROVED"
				if action == "reject" {
					approval.State = "REJECTED"
				}
				approval.Decision = &domain.ReleaseApprovalDecision{ActorID: actor, At: now.UTC().Format(time.RFC3339Nano), Reason: input.Reason, Source: source.Source, Roles: source.Roles}
			}
		}
		allApproved = allApproved && approval.State == "APPROVED"
	}
	if action == "reject" {
		order.State = "REJECTED"
		return s.ReleaseTargets(ctx, order.ID)
	}
	if allApproved {
		order.State = "APPROVED"
	}
	return nil
}
