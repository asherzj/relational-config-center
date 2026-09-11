package domain

// ReleaseItem records intent and the server-verified record baseline. A nil
// Before means no record, while a nil field value means SQL NULL. Content omits
// unsupplied fields. Automatic fields remain deferred until actual publication.
type ReleaseItem struct {
	Publication           *PublicationCommand `json:"publication,omitempty"`
	Rollback              *PublicationCommand `json:"rollback,omitempty"`
	TableName             string              `json:"table_name"`
	DetailID              string              `json:"detail_id"`
	ConcurrencyKeys       [][]byte            `json:"concurrency_keys,omitempty"`
	Operation             string              `json:"operation"`
	ID                    *JSONString         `json:"id"`
	ExpectedRecordVersion string              `json:"expected_record_version"`
	Content               MutationContent     `json:"content"`
	Before                Row                 `json:"before"`
	RecordTable           string              `json:"record_table,omitempty"`
	RecordKey             []byte              `json:"record_key,omitempty"`
	Fields                []ReleaseField      `json:"fields"`
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
	TableNames      []string                `json:"table_names,omitempty"`
	ApprovalSources []ReleaseApprovalSource `json:"approval_sources,omitempty"`
	RelatedOrderID  string                  `json:"related_order_id,omitempty"`
	ExecutionID     string                  `json:"execution_id,omitempty"`
	Action          string                  `json:"action"`
	ActorID         string                  `json:"actor_id"`
	At              string                  `json:"at"`
	Version         string                  `json:"version"`
	Reason          string                  `json:"reason"`
}

type ReleaseOrder struct {
	ReleaseType       ReleaseType                         `json:"release_type"`
	TableFlows        []ReleaseTableFlow                  `json:"table_flows"`
	MissingFlowTables []string                            `json:"missing_flow_tables"`
	Approvals         []ReleaseTableApproval              `json:"approvals"`
	ApprovalContext   ReleaseApprovalContext              `json:"approval_context"`
	FrozenTables      map[string]ReleaseExecutionSnapshot `json:"frozen_tables,omitempty"`
	TableNames        []string                            `json:"table_names"`
	Title             string                              `json:"title"`
	Executions        []ReleaseExecution                  `json:"executions"`
	CopiedFromID      string                              `json:"copied_from_id,omitempty"`
	FrozenDigest      string                              `json:"frozen_digest,omitempty"`
	ID                string                              `json:"id"`
	ApplicantID       string                              `json:"applicant_id"`
	State             string                              `json:"state"`
	Version           string                              `json:"version"`
	Items             []ReleaseItem                       `json:"items"`
	History           []ReleaseEvent                      `json:"history"`
	CreatedAt         string                              `json:"created_at"`
	UpdatedAt         string                              `json:"updated_at"`
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
	TableName string              `json:"table_name"`
	Format    string              `json:"format"`
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
	ReleaseType       ReleaseType            `json:"release_type"`
	TableFlows        []ReleaseTableFlow     `json:"table_flows"`
	MissingFlowTables []string               `json:"missing_flow_tables"`
	Approvals         []ReleaseTableApproval `json:"approvals"`
	ApprovalContext   ReleaseApprovalContext `json:"approval_context"`
	TableNames        []string               `json:"table_names"`
	Title             string                 `json:"title"`
	ID                string                 `json:"id"`
	ApplicantID       string                 `json:"applicant_id"`
	State             string                 `json:"state"`
	Version           string                 `json:"version"`
	CreatedAt         string                 `json:"created_at"`
	UpdatedAt         string                 `json:"updated_at"`
	ItemCount         int                    `json:"item_count"`
	OperationCounts   map[string]int         `json:"operation_counts"`
}

func (order ReleaseOrder) Summary() ReleaseOrderSummary {
	result := ReleaseOrderSummary{ReleaseType: order.ReleaseType, TableFlows: order.TableFlows, MissingFlowTables: order.MissingFlowTables, Approvals: order.Approvals, ApprovalContext: order.ApprovalContext, TableNames: order.TableNames, Title: order.Title, ID: order.ID, ApplicantID: order.ApplicantID, State: order.State, Version: order.Version, CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt, ItemCount: len(order.Items), OperationCounts: map[string]int{}}
	for _, item := range order.Items {
		result.OperationCounts[item.Operation]++
	}
	return result
}
