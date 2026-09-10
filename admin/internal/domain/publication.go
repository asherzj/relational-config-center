package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// PublicationResult records database commit progress only. There is no delivery
// worker or consumer acknowledgement in this Admin single-source protocol.
type PublicationResult struct {
	TableVersions map[string]string              `json:"table_versions"`
	Notifications map[string]RefreshNotification `json:"notifications"`
	ExecutionID   string                         `json:"execution_id"`
	Kind          string                         `json:"kind"`
	TableVersion  string                         `json:"table_version"`
	PublisherID   string                         `json:"publisher_id"`
	ExecutedAt    string                         `json:"executed_at"`
	Commands      []PublicationCommand           `json:"commands"`
	Notification  RefreshNotification            `json:"notification"`
}
type RefreshNotification struct {
	TableName    string `json:"table_name"`
	ID           string `json:"id"`
	TableVersion string `json:"table_version"`
	Status       string `json:"status"`
}
type PublicationCommand struct {
	ExecutionID   string       `json:"execution_id"`
	ExecutionKind string       `json:"execution_kind"`
	OrderID       string       `json:"order_id"`
	Sequence      string       `json:"sequence"`
	TableName     string       `json:"table_name"`
	TableVersion  string       `json:"table_version"`
	Operation     string       `json:"operation"`
	ID            string       `json:"id"`
	RecordVersion string       `json:"record_version"`
	Before        CanonicalRow `json:"before"`
	Final         CanonicalRow `json:"final"`
}

// VerifyPublication checks stored rows against the order's independently frozen
// schema, not a digest supplied by either row. Readers fail closed on corruption.
func (order ReleaseOrder) VerifyPublication() error {
	if err := order.verifyExecution(order.Publication, false); err != nil {
		return err
	}
	if order.Rollback != nil {
		if order.State != "ROLLED_BACK" {
			return ErrCanonicalRow
		}
		if err := order.verifyExecution(order.Rollback, true); err != nil {
			return err
		}
	} else if order.State == "ROLLED_BACK" {
		return ErrCanonicalRow
	}
	expected := 0
	for _, result := range []*PublicationResult{order.Publication, order.Rollback} {
		if result == nil {
			continue
		}
		if expected >= len(order.Executions) || !order.Executions[expected].Matches(*result) {
			return ErrCanonicalRow
		}
		expected++
	}
	if expected != len(order.Executions) {
		return ErrCanonicalRow
	}
	return nil
}

func (order ReleaseOrder) verifyExecution(result *PublicationResult, rollback bool) error {
	if result == nil {
		if !rollback && (order.State == "SUCCEEDED" || order.State == "COMPLETED" || order.State == "ROLLED_BACK") {
			return ErrCanonicalRow
		}
		return nil
	}
	if len(order.FrozenTables) == 0 || len(result.Commands) != len(order.Items) || len(result.Commands) == 0 || result.Notification.ID != result.ExecutionID || result.Notification.TableVersion != result.TableVersion || result.Notification.Status != "NOT_CONNECTED" {
		return ErrCanonicalRow
	}
	kind := "PUBLICATION"
	if rollback {
		kind = "ROLLBACK"
	}
	if result.Kind != kind || result.ExecutionID != order.ID+":"+kind {
		return ErrCanonicalRow
	}
	if len(result.TableVersions) != len(order.FrozenTables) || len(result.Notifications) != len(order.FrozenTables) {
		return ErrCanonicalRow
	}
	type verifiedSchema struct {
		digest  string
		columns []ExecutionColumn
	}
	schemas := make(map[string]verifiedSchema, len(order.FrozenTables))
	for table, frozen := range order.FrozenTables {
		if frozen.Schema.TableName != table {
			return ErrCanonicalRow
		}
		encoded, err := json.Marshal(frozen.Schema)
		if err != nil {
			return ErrCanonicalRow
		}
		sum := sha256.Sum256(encoded)
		columns, err := frozen.Schema.Columns()
		if err != nil {
			return err
		}
		schemas[table] = verifiedSchema{digest: hex.EncodeToString(sum[:]), columns: columns}
		notice, found := result.Notifications[table]
		if !found || notice.ID != result.ExecutionID || notice.TableName != table || notice.TableVersion != result.TableVersions[table] || notice.Status != "NOT_CONNECTED" {
			return ErrCanonicalRow
		}
	}
	for i, command := range result.Commands {
		item := order.Items[i]
		if rollback {
			item = order.Items[len(order.Items)-1-i]
		}
		schema, found := schemas[item.TableName]
		if !found {
			return ErrCanonicalRow
		}
		digest, columns := schema.digest, schema.columns
		operation := item.Operation
		if rollback {
			operation = order.Items[len(order.Items)-1-i].Operation
			if operation == "ADD" {
				operation = "DELETE"
			} else if operation == "DELETE" {
				operation = "ADD"
			}
		}
		if command.ExecutionID != result.ExecutionID || command.ExecutionKind != result.Kind || command.OrderID != order.ID || command.TableName != item.TableName || command.TableVersion != result.TableVersions[item.TableName] || command.Operation != operation || command.Before.Deleted != (command.Operation == "ADD") || command.Final.Deleted != (command.Operation == "DELETE") {
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
				column := columns[j]
				if field.Name != column.Name || field.Type != column.Type {
					return ErrCanonicalRow
				}
			}
		}
	}
	return nil
}

// ReleaseExecution is a successful whole-order summary. The actual rows belong
// to Release Change Details and are assembled into PublicationResult for reads.
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
	Notification    RefreshNotification            `json:"notification"`
}

func SummarizeExecution(result PublicationResult) ReleaseExecution {
	execution := ReleaseExecution{ID: result.ExecutionID, Kind: result.Kind, ActorID: result.PublisherID, ExecutedAt: result.ExecutedAt, TableVersions: map[string]string{}, OperationCounts: map[string]int{}, ItemCount: len(result.Commands), Outcome: "SUCCEEDED", Notifications: result.Notifications, Notification: result.Notification}
	for _, command := range result.Commands {
		execution.TableVersions[command.TableName] = command.TableVersion
		execution.OperationCounts[command.Operation]++
	}
	return execution
}
func (execution ReleaseExecution) Matches(result PublicationResult) bool {
	actual, _ := json.Marshal(SummarizeExecution(result))
	stored, _ := json.Marshal(execution)
	return string(actual) == string(stored)
}
