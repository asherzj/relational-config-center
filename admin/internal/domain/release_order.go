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
	RecordTable           string          `json:"record_table,omitempty"`
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
	Publication  *PublicationResult        `json:"publication,omitempty"`
	CopiedFromID string                    `json:"copied_from_id,omitempty"`
	Frozen       *ReleaseExecutionSnapshot `json:"frozen,omitempty"`
	FrozenDigest string                    `json:"frozen_digest,omitempty"`
	ID           string                    `json:"id"`
	TableName    string                    `json:"table_name"`
	ApplicantID  string                    `json:"applicant_id"`
	State        string                    `json:"state"`
	Version      string                    `json:"version"`
	Items        []ReleaseItem             `json:"items"`
	History      []ReleaseEvent            `json:"history"`
	CreatedAt    string                    `json:"created_at"`
	UpdatedAt    string                    `json:"updated_at"`
}

type RecordBaseline struct {
	GeneratesIDOnInsert bool
	TableName           string
	Row                 Row
	Key                 []byte
	Version             string
}

type ReleaseFilter struct {
	TableName, ApplicantID, State, ID, After string
	Limit                                    int
}

// Execution metadata retains NULL separately from text. Sections have a fixed,
// versioned projection and exclude display descriptions and mutable statistics.
type ExecutionMetadata struct {
	Name string      `json:"name"`
	Rows [][]*string `json:"rows"`
}
type TableExecutionSchema struct {
	Format    string              `json:"format"`
	TableName string              `json:"table_name"`
	Sections  []ExecutionMetadata `json:"sections"`
}
type ReleaseMutationSemantics struct {
	TypeCode            string  `json:"type_code"`
	AllowAdd            bool    `json:"allow_add"`
	AllowModify         bool    `json:"allow_modify"`
	AllowDelete         bool    `json:"allow_delete"`
	CreateOperatorField *string `json:"create_operator_field"`
	CreateTimeField     *string `json:"create_time_field"`
	ModifyOperatorField *string `json:"modify_operator_field"`
	ModifyTimeField     *string `json:"modify_time_field"`
}
type ReleaseExecutionSnapshot struct {
	Schema   TableExecutionSchema     `json:"schema"`
	Mutation ReleaseMutationSemantics `json:"mutation"`
}

// ActiveTarget is a database comparison identity, never a request spelling.
type ActiveTarget struct {
	TableName string
	RecordKey []byte
}

// NewReleaseMutationSemantics records only execution meaning, excluding display metadata.
func NewReleaseMutationSemantics(policy MutationPolicy) ReleaseMutationSemantics {
	return ReleaseMutationSemantics{TypeCode: policy.TypeCode, AllowAdd: policy.AllowAdd, AllowModify: policy.AllowModify, AllowDelete: policy.AllowDelete, CreateOperatorField: policy.CreateOperatorField, CreateTimeField: policy.CreateTimeField, ModifyOperatorField: policy.ModifyOperatorField, ModifyTimeField: policy.ModifyTimeField}
}
