package domain

// ReleaseItem records intent and the server-verified record baseline. A nil
// Before means no record, while a nil field value means SQL NULL. Content omits
// unsupplied fields. Automatic fields remain deferred until actual publication.
type ReleaseItem struct {
	Operation             string          `json:"operation"`
	ID                    *JSONString     `json:"id"`
	ExpectedRecordVersion string          `json:"expected_record_version"`
	Content               MutationContent `json:"content"`
	Before                Row             `json:"before"`
	RecordKey             []byte          `json:"record_key,omitempty"`
	Fields                []ReleaseField  `json:"fields"`
}

type ReleaseField struct {
	Nullable      bool        `json:"nullable"`
	Editable      bool        `json:"editable"`
	Name          string      `json:"name"`
	Type          ColumnType  `json:"type"`
	BeforeState   string      `json:"before_state"`
	Before        *JSONString `json:"before"`
	ProposedState string      `json:"proposed_state"`
	Proposed      *JSONString `json:"proposed"`
}

type ReleaseEvent struct {
	Action  string `json:"action"`
	ActorID string `json:"actor_id"`
	At      string `json:"at"`
	Version string `json:"version"`
	Reason  string `json:"reason"`
}

type ReleaseOrder struct {
	ID          string         `json:"id"`
	TableName   string         `json:"table_name"`
	ApplicantID string         `json:"applicant_id"`
	State       string         `json:"state"`
	Version     string         `json:"version"`
	Items       []ReleaseItem  `json:"items"`
	History     []ReleaseEvent `json:"history"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

type RecordBaseline struct {
	Row     Row
	Key     []byte
	Version string
}

type ReleaseFilter struct {
	TableName, ApplicantID, State, ID, After string
	Limit                                    int
}
