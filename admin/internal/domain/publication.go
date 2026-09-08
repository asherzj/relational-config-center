package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// PublicationResult records database commit progress only. There is no delivery
// worker or consumer acknowledgement in this Admin single-source protocol.
type PublicationResult struct {
	TableVersion string               `json:"table_version"`
	PublisherID  string               `json:"publisher_id"`
	ExecutedAt   string               `json:"executed_at"`
	Commands     []PublicationCommand `json:"commands"`
	Notification RefreshNotification  `json:"notification"`
}
type RefreshNotification struct {
	ID           string `json:"id"`
	TableVersion string `json:"table_version"`
	Status       string `json:"status"`
}
type PublicationCommand struct {
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
	result := order.Publication
	if result == nil {
		if order.State == "SUCCEEDED" || order.State == "ROLLED_BACK" {
			return ErrCanonicalRow
		}
		return nil
	}
	if order.Frozen == nil || len(result.Commands) != len(order.Items) || len(result.Commands) == 0 || result.Notification.ID != order.ID || result.Notification.TableVersion != result.TableVersion || result.Notification.Status != "NOT_CONNECTED" {
		return ErrCanonicalRow
	}
	schemaJSON, err := json.Marshal(order.Frozen.Schema)
	if err != nil {
		return ErrCanonicalRow
	}
	schemaSum := sha256.Sum256(schemaJSON)
	digest := hex.EncodeToString(schemaSum[:])
	columns, err := order.Frozen.Schema.Columns()
	if err != nil {
		return err
	}
	for i, command := range result.Commands {
		if command.OrderID != order.ID || command.TableName != order.Frozen.Schema.TableName || command.TableVersion != result.TableVersion || command.Operation != order.Items[i].Operation || command.Before.Deleted != (command.Operation == "ADD") || command.Final.Deleted != (command.Operation == "DELETE") {
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
