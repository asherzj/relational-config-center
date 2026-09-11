package domain

// The submitted role identities and completed decisions are immutable facts.
// Live eligibility is a separate read and is never persisted in their place.
type ReleaseTableApproval struct {
	TableName string                   `json:"table_name"`
	Roles     []ApprovalRoleIdentity   `json:"roles"`
	State     string                   `json:"state"`
	Decision  *ReleaseApprovalDecision `json:"decision,omitempty"`
}
type ReleaseApprovalDecision struct {
	ActorID string                 `json:"actor_id"`
	At      string                 `json:"at"`
	Reason  string                 `json:"reason"`
	Source  string                 `json:"source"`
	Roles   []ApprovalRoleIdentity `json:"roles"`
}
type ReleaseApprovalSource struct {
	TableName string                 `json:"table_name"`
	Source    string                 `json:"source"`
	Roles     []ApprovalRoleIdentity `json:"roles"`
}
type ReleaseApprovalTableStatus struct {
	TableName  string `json:"table_name"`
	Mode       string `json:"mode"`
	Reason     string `json:"reason"`
	CanApprove bool   `json:"can_approve"`
}
type ReleaseApprovalContext struct {
	Revision         string                       `json:"revision"`
	Tables           []ReleaseApprovalTableStatus `json:"tables"`
	ApprovableTables []string                     `json:"approvable_tables"`
}

// These minimal directory records contain only qualification facts, never credentials.
type ApprovalAccount struct {
	ID             string
	Enabled        bool
	Roles          AccountRoles
	RoleVersion    uint64
	SessionVersion uint64
}
type ReleaseApprovalEnvironment struct {
	Approvals []ReleaseTableApproval
	Roles     []ApprovalRole
	Accounts  []ApprovalAccount
}

// Approval facts must cover the actual table set exactly. Older submitted
// documents without this responsibility snapshot are not executable orders.
func (order ReleaseOrder) HasCompleteApproval() bool {
	if len(order.TableNames) == 0 || len(order.Approvals) != len(order.TableNames) {
		return false
	}
	tables := map[string]bool{}
	for _, table := range order.TableNames {
		tables[table] = true
	}
	for _, approval := range order.Approvals {
		if !tables[approval.TableName] || approval.State != "APPROVED" || approval.Decision == nil {
			return false
		}
		delete(tables, approval.TableName)
	}
	return len(tables) == 0
}
