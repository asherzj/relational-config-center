package domain

import (
	"context"
	"errors"
	"time"
)

var ErrApprovalRoleFields = errors.New("invalid approval role fields")
var ErrApprovalRoleNotFound = errors.New("approval role not found")
var ErrApprovalRoleReferenced = errors.New("approval role was referenced")

var ErrApprovalRoleNotSaved = errors.New("approval role transaction was not committed")

var ErrApprovalRoleVersion = errors.New("approval role version conflict")

type ApprovalMember struct {
	ID          string
	Username    string
	DisplayName string
	Enabled     bool
}

type ApprovalRole struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	Version     uint64
	Creator     string
	Modifier    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Members     []ApprovalMember
	Referenced  bool
	Deleted     bool
}

type ApprovalRoleChange struct {
	ActorID         string
	ID              string
	Name            string
	Description     string
	Enabled         bool
	MemberIDs       []string
	ExpectedVersion uint64
	RequestKey      string
}

type ApprovalRoleRepository interface {
	TableApprovalRepository
	ListApprovalRoles(context.Context, string, string, int) ([]ApprovalRole, error)
	GetApprovalRole(context.Context, string) (ApprovalRole, error)
	SaveApprovalRole(context.Context, ApprovalRoleChange) (ApprovalRole, error)
	DeleteApprovalRole(context.Context, ApprovalRoleChange) (ApprovalRole, error)
}
