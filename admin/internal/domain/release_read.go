package domain

// ReleaseHeader is the workflow and successful execution catalog. It never
// contains application fields or actual per-record results.
type ReleaseHeader struct {
	ReleaseOrderSummary
	CopiedFromID string                              `json:"copied_from_id,omitempty"`
	FrozenDigest string                              `json:"frozen_digest,omitempty"`
	FrozenTables map[string]ReleaseExecutionSnapshot `json:"frozen_tables,omitempty"`
	History      []ReleaseEvent                      `json:"history"`
	Executions   []ReleaseExecution                  `json:"executions"`
}

func (header ReleaseHeader) Workflow() ReleaseOrder {
	return ReleaseOrder{RollbackTableFlows: header.RollbackTableFlows, EmergencyReason: header.EmergencyReason, ReleaseType: header.ReleaseType, TableFlows: header.TableFlows, MissingFlowTables: header.MissingFlowTables, Approvals: header.Approvals, ApprovalContext: header.ApprovalContext, ID: header.ID, Title: header.Title, TableNames: header.TableNames, ApplicantID: header.ApplicantID, State: header.State, Version: header.Version, CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt, CopiedFromID: header.CopiedFromID, FrozenDigest: header.FrozenDigest, FrozenTables: header.FrozenTables, History: header.History, Executions: header.Executions}
}

// Offset and NextOffset are zero-based original detail positions. Results stay
// attached to that original position even when rollback executed in reverse.
type ReleaseDetailPage struct {
	OrderID    string        `json:"order_id"`
	Version    string        `json:"version"`
	ItemCount  int           `json:"item_count"`
	Offset     int           `json:"offset"`
	NextOffset *int          `json:"next_offset"`
	Items      []ReleaseItem `json:"items"`
}
