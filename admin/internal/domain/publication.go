package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
)

// PublicationCommit is the transient result of one shared atomic executor call.
// Persistent per-item results belong to ReleaseItem; only the summary is retained
// by ReleaseExecution. It is never a whole-result transport or storage document.
type PublicationCommit struct {
	TableVersions map[string]string              `json:"table_versions"`
	Notifications map[string]RefreshNotification `json:"notifications"`
	ExecutionID   string                         `json:"execution_id"`
	Kind          string                         `json:"kind"`
	PublisherID   string                         `json:"publisher_id"`
	ExecutedAt    string                         `json:"executed_at"`
	Commands      []PublicationCommand           `json:"commands"`
}
type RefreshNotification struct {
	TableVersion string `json:"table_version"`
	TableName    string `json:"table_name"`
	ID           string `json:"id"`
	Status       string `json:"status"`
}
type PublicationCommand struct {
	DetailID      string       `json:"detail_id"`
	TableVersion  string       `json:"table_version"`
	ExecutionID   string       `json:"execution_id"`
	ExecutionKind string       `json:"execution_kind"`
	OrderID       string       `json:"order_id"`
	Sequence      string       `json:"sequence"`
	TableName     string       `json:"table_name"`
	Operation     string       `json:"operation"`
	ID            string       `json:"id"`
	RecordVersion string       `json:"record_version"`
	Before        CanonicalRow `json:"before"`
	Final         CanonicalRow `json:"final"`
}

// VerifyPublication checks stored rows against the order's independently frozen
// schema, not a digest supplied by either row. Readers fail closed on corruption.
func (order ReleaseOrder) VerifyPublication() error {
	if err := order.VerifyExecutionSummaries(); err != nil {
		return err
	}
	for _, execution := range order.Executions {
		if execution.ItemCount != len(order.Items) {
			return ErrCanonicalRow
		}
		counts := map[string]int{}
		for _, item := range order.Items {
			if err := order.VerifyExecutionDetail(execution, item); err != nil {
				return err
			}
			command := item.Publication
			if execution.Kind == "ROLLBACK" {
				command = item.Rollback
			}
			counts[command.Operation]++
		}
		if !maps.Equal(counts, execution.OperationCounts) {
			return ErrCanonicalRow
		}
	}
	for _, item := range order.Items {
		if (item.Publication != nil) != (len(order.Executions) >= 1) || (item.Rollback != nil) != (len(order.Executions) == 2) {
			return ErrCanonicalRow
		}
	}
	return nil
}

func (order ReleaseOrder) VerifyExecutionSummaries() error {
	expected := 0
	if order.State == "SUCCEEDED" || order.State == "COMPLETED" {
		expected = 1
	}
	if order.State == "ROLLED_BACK" {
		expected = 2
	}
	if len(order.Executions) != expected {
		return ErrCanonicalRow
	}
	for index, execution := range order.Executions {
		kind := "PUBLICATION"
		if index == 1 {
			kind = "ROLLBACK"
		}
		if execution.Kind != kind || execution.ID != order.ID+":"+kind || execution.Outcome != "SUCCEEDED" || execution.ItemCount < 1 || execution.ItemCount > 1000 || len(execution.TableVersions) != len(order.FrozenTables) || len(execution.Notifications) != len(order.FrozenTables) {
			return ErrCanonicalRow
		}
		total := 0
		for operation, count := range execution.OperationCounts {
			if (operation != "ADD" && operation != "MODIFY" && operation != "DELETE") || count < 1 {
				return ErrCanonicalRow
			}
			total += count
		}
		if total != execution.ItemCount || len(order.FrozenTables) == 0 {
			return ErrCanonicalRow
		}
		for table := range order.FrozenTables {
			notice, found := execution.Notifications[table]
			if !found || notice.ID != execution.ID || notice.TableName != table || notice.TableVersion != execution.TableVersions[table] || notice.Status != "NOT_CONNECTED" {
				return ErrCanonicalRow
			}
		}
	}
	return nil
}

// VerifyExecutionDetail validates one persisted result against independently
// frozen schema and successful execution ownership; paged readers use this too.
func (order ReleaseOrder) VerifyExecutionDetail(execution ReleaseExecution, item ReleaseItem) error {
	command := item.Publication
	if execution.Kind == "ROLLBACK" {
		command = item.Rollback
	}
	if command == nil || command.DetailID == "" || command.DetailID != item.DetailID || execution.Kind == "ROLLBACK" && (item.Publication == nil || command.ID != item.Publication.ID) {
		return ErrCanonicalRow
	}
	frozen, found := order.FrozenTables[item.TableName]
	if !found || frozen.Schema.TableName != item.TableName {
		return ErrCanonicalRow
	}
	encoded, err := json.Marshal(frozen.Schema)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	columns, err := frozen.Schema.Columns()
	if err != nil {
		return err
	}
	operation := item.Operation
	if execution.Kind == "ROLLBACK" {
		if operation == "ADD" {
			operation = "DELETE"
		} else if operation == "DELETE" {
			operation = "ADD"
		}
	}
	if command.ExecutionID != execution.ID || command.ExecutionKind != execution.Kind || command.OrderID != order.ID || command.TableName != item.TableName || command.TableVersion != execution.TableVersions[item.TableName] || command.Operation != operation || command.Before.Deleted != (command.Operation == "ADD") || command.Final.Deleted != (command.Operation == "DELETE") {
		return ErrCanonicalRow
	}
	identityRow := command.Final
	if command.Final.Deleted {
		identityRow = command.Before
	}
	actualID, err := identityRow.RecordID()
	if err != nil || actualID != command.ID {
		return ErrCanonicalRow
	}
	for _, row := range []CanonicalRow{command.Before, command.Final} {
		if row.Verify(digest) != nil {
			return ErrCanonicalRow
		}
		if row.Deleted {
			continue
		}
		if len(row.Fields) != len(columns) {
			return ErrCanonicalRow
		}
		for j, field := range row.Fields {
			if field.Name != columns[j].Name || field.Type != columns[j].Type {
				return ErrCanonicalRow
			}
		}
	}
	return nil
}

// ReleaseExecution is a successful whole-order summary. The actual rows belong
// to Release Change Details.
type ReleaseExecution struct {
	Notifications   map[string]RefreshNotification `json:"notifications"`
	ID              string                         `json:"id"`
	Kind            string                         `json:"kind"`
	ActorID         string                         `json:"actor_id"`
	ExecutedAt      string                         `json:"executed_at"`
	TableVersions   map[string]string              `json:"table_versions"`
	OperationCounts map[string]int                 `json:"operation_counts"`
	ItemCount       int                            `json:"item_count"`
	Outcome         string                         `json:"outcome"`
}

func SummarizeExecution(result PublicationCommit) ReleaseExecution {
	execution := ReleaseExecution{ID: result.ExecutionID, Kind: result.Kind, ActorID: result.PublisherID, ExecutedAt: result.ExecutedAt, TableVersions: map[string]string{}, OperationCounts: map[string]int{}, ItemCount: len(result.Commands), Outcome: "SUCCEEDED", Notifications: result.Notifications}
	for _, command := range result.Commands {
		execution.TableVersions[command.TableName] = command.TableVersion
		execution.OperationCounts[command.Operation]++
	}
	return execution
}

// ApplyExecution deposits committed rows in the original ordered details.
// PublicationCommit exists only while the shared atomic executor is running.
func (order *ReleaseOrder) ApplyExecution(result PublicationCommit) error {
	if len(result.Commands) != len(order.Items) {
		return ErrCanonicalRow
	}
	for index := range result.Commands {
		command := result.Commands[index]
		position := index
		if result.Kind == "ROLLBACK" {
			position = len(order.Items) - 1 - index
			order.Items[position].Rollback = &command
		} else {
			order.Items[position].Publication = &command
		}
	}
	order.Executions = append(order.Executions, SummarizeExecution(result))
	return nil
}
