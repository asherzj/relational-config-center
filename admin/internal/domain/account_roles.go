package domain

import "errors"

// AccountRoles is a combination of global grants. Every valid role can read.
type AccountRoles uint8

const (
	RoleViewer AccountRoles = 1 << iota
	RoleEditor
	RoleApprover
	RolePublisher
	RoleAdmin
)

var ErrLastAdministrator = errors.New("cannot remove the last enabled administrator")

var ErrRoleIdempotencyConflict = errors.New("role request key reused for different content")

var ErrPermissionDenied = errors.New("permission denied")

func (roles AccountRoles) Allows(required AccountRoles) bool {
	return roles >= RoleViewer && roles < 32 && (required == RoleViewer || roles&RoleAdmin != 0 || roles&required != 0)
}

func (roles AccountRoles) Names() []string {
	names := make([]string, 0, 5)
	for index, name := range []string{"VIEWER", "EDITOR", "APPROVER", "PUBLISHER", "ADMIN"} {
		if roles&(1<<index) != 0 {
			names = append(names, name)
		}
	}
	return names
}

var (
	ErrInvalidRoles        = errors.New("invalid account roles")
	ErrRoleVersionConflict = errors.New("account roles changed")
)

func ParseAccountRoles(names []string) (AccountRoles, error) {
	var roles AccountRoles
	if len(names) == 0 || len(names) > 5 {
		return 0, ErrInvalidRoles
	}
	for _, name := range names {
		var role AccountRoles
		switch name {
		case "VIEWER":
			role = RoleViewer
		case "EDITOR":
			role = RoleEditor
		case "APPROVER":
			role = RoleApprover
		case "PUBLISHER":
			role = RolePublisher
		case "ADMIN":
			role = RoleAdmin
		default:
			return 0, ErrInvalidRoles
		}
		if roles&role != 0 {
			return 0, ErrInvalidRoles
		}
		roles |= role
	}
	return roles, nil
}
