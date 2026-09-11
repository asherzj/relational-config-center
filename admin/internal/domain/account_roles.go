package domain

import "errors"

// AccountRoles is a combination of global grants. Every valid role can read.
type AccountRoles uint8

const (
	RoleViewer AccountRoles = 1
	RoleEditor AccountRoles = 2
	// Bit 4 is permanently reserved for historical APPROVER grants.
	RolePublisher AccountRoles = 8
	RoleAdmin     AccountRoles = 16
)

var ErrLastAdministrator = errors.New("cannot remove the last enabled administrator")

var ErrRoleIdempotencyConflict = errors.New("role request key reused for different content")

var ErrPermissionDenied = errors.New("permission denied")

func (roles AccountRoles) Allows(required AccountRoles) bool {
	return roles.Valid() && required.Valid() && (required == RoleViewer || roles&RoleAdmin != 0 || roles&required != 0)
}

// Valid rejects retired or unknown current grants instead of translating them.
func (roles AccountRoles) Valid() bool {
	return roles != 0 && roles & ^(RoleViewer|RoleEditor|RolePublisher|RoleAdmin) == 0
}

func (roles AccountRoles) Names() []string {
	if !roles.Valid() {
		return []string{}
	}
	names := make([]string, 0, 4)
	for index, name := range []string{"VIEWER", "EDITOR", "", "PUBLISHER", "ADMIN"} {
		if name != "" && roles&(1<<index) != 0 {
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
	if len(names) == 0 || len(names) > 4 {
		return 0, ErrInvalidRoles
	}
	for _, name := range names {
		var role AccountRoles
		switch name {
		case "VIEWER":
			role = RoleViewer
		case "EDITOR":
			role = RoleEditor
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
