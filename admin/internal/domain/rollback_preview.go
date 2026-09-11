package domain

// QuickRollbackPreview is the acknowledged restoration intent and its saved
// emergency workflow. It does not authorize execution against later data.
type QuickRollbackPreview struct {
	OrderID         string             `json:"order_id"`
	ExpectedVersion string             `json:"expected_version"`
	PreviewDigest   string             `json:"preview_digest"`
	ReleaseType     ReleaseType        `json:"release_type"`
	TableFlows      []ReleaseTableFlow `json:"table_flows"`
	Items           []ReleaseItem      `json:"items"`
}
