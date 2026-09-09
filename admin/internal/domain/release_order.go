package domain

// ReleaseItem records intent and the server-verified record baseline. A nil
// Before means no record, while a nil field value means SQL NULL. Content omits
// unsupplied fields. Automatic fields remain deferred until actual publication.
type ReleaseItem struct {
	DetailID              string          `json:"detail_id"`
	TableName             string          `json:"table_name"`
	ConcurrencyKeys       [][]byte        `json:"concurrency_keys,omitempty"`
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
	RelatedOrderID string `json:"related_order_id,omitempty"`
	Action         string `json:"action"`
	ActorID        string `json:"actor_id"`
	At             string `json:"at"`
	Version        string `json:"version"`
	Reason         string `json:"reason"`
}

type ReleaseOrder struct {
	Title           string                    `json:"title"`
	RollbackOfID    string                    `json:"rollback_of_id,omitempty"`
	RollbackOrderID string                    `json:"rollback_order_id,omitempty"`
	RollbackPending bool                      `json:"rollback_pending,omitempty"`
	Rollback        *PublicationResult        `json:"rollback,omitempty"`
	Executions      []ReleaseExecution        `json:"executions,omitempty"`
	Publication     *PublicationResult        `json:"publication,omitempty"`
	CopiedFromID    string                    `json:"copied_from_id,omitempty"`
	Frozen          *ReleaseExecutionSnapshot `json:"frozen,omitempty"`
	FrozenDigest    string                    `json:"frozen_digest,omitempty"`
	ID              string                    `json:"id"`
	TableName       string                    `json:"table_name"`
	ApplicantID     string                    `json:"applicant_id"`
	State           string                    `json:"state"`
	Version         string                    `json:"version"`
	Items           []ReleaseItem             `json:"items"`
	History         []ReleaseEvent            `json:"history"`
	CreatedAt       string                    `json:"created_at"`
	UpdatedAt       string                    `json:"updated_at"`
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

// ExecutionColumn names the fixed column projection held by the execution
// snapshot. Consumers do not need to interpret its persisted positional format.
type ExecutionColumn struct {
	Name, Type                               string
	Charset, Collation, GenerationExpression *string
}

func (schema TableExecutionSchema) Columns() ([]ExecutionColumn, error) {
	for _, section := range schema.Sections {
		if section.Name != "columns" {
			continue
		}
		columns := make([]ExecutionColumn, 0, len(section.Rows))
		for _, row := range section.Rows {
			if len(row) != 10 || row[0] == nil || row[2] == nil {
				return nil, ErrCanonicalRow
			}
			columns = append(columns, ExecutionColumn{Name: *row[0], Type: *row[2], Charset: row[5], Collation: row[6], GenerationExpression: row[8]})
		}
		return columns, nil
	}
	return nil, ErrCanonicalRow
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
	ItemIndex int
	TableName string
	RecordKey []byte
}

// NewReleaseMutationSemantics records only execution meaning, excluding display metadata.
func NewReleaseMutationSemantics(policy MutationPolicy) ReleaseMutationSemantics {
	return ReleaseMutationSemantics{TypeCode: policy.TypeCode, AllowAdd: policy.AllowAdd, AllowModify: policy.AllowModify, AllowDelete: policy.AllowDelete, CreateOperatorField: policy.CreateOperatorField, CreateTimeField: policy.CreateTimeField, ModifyOperatorField: policy.ModifyOperatorField, ModifyTimeField: policy.ModifyTimeField}
}

// ReleaseOrderSummary carries bounded catalog information; complete intent and
// verified publication history are available through the order detail.
type ReleaseOrderSummary struct {
	Title           string         `json:"title"`
	RollbackOfID    string         `json:"rollback_of_id,omitempty"`
	RollbackOrderID string         `json:"rollback_order_id,omitempty"`
	RollbackPending bool           `json:"rollback_pending,omitempty"`
	ID              string         `json:"id"`
	TableName       string         `json:"table_name"`
	ApplicantID     string         `json:"applicant_id"`
	State           string         `json:"state"`
	Version         string         `json:"version"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	ItemCount       int            `json:"item_count"`
	OperationCounts map[string]int `json:"operation_counts"`
}

func (order ReleaseOrder) Summary() ReleaseOrderSummary {
	result := ReleaseOrderSummary{Title: order.Title, RollbackOfID: order.RollbackOfID, RollbackOrderID: order.RollbackOrderID, RollbackPending: order.RollbackPending, ID: order.ID, TableName: order.TableName, ApplicantID: order.ApplicantID, State: order.State, Version: order.Version, CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt, ItemCount: len(order.Items), OperationCounts: map[string]int{}}
	for _, item := range order.Items {
		result.OperationCounts[item.Operation]++
	}
	return result
}
