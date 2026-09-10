package domain

import (
	"context"
	"errors"
)

var ErrTableApprovalVersion = errors.New("table approval assignment changed")

type ApprovalRoleIdentity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TableApprovalAssignment struct {
	TableName string                 `json:"table_name"`
	Version   string                 `json:"version"`
	RoleIDs   []string               `json:"role_ids"`
	Roles     []ApprovalRoleIdentity `json:"roles"`
}

type TableApprovalChange struct {
	ActorID, TableName, ExpectedVersion, RequestKey string
	RoleIDs                                         []string
}

type TableApprovalRepository interface {
	ReadTableApproval(context.Context, string) (TableApprovalAssignment, error)
	SaveTableApproval(context.Context, TableApprovalChange) (TableApprovalAssignment, error)
}
