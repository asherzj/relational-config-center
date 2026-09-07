package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrReleaseNotFound            = errors.New("release order not found")
	ErrReleaseInvalid             = errors.New("invalid release order")
	ErrReleaseVersionConflict     = errors.New("release order version conflict")
	ErrReleaseState               = errors.New("release order state does not allow this action")
	ErrReleaseIdempotencyConflict = errors.New("release request identifier already used with different content")
	ErrReleaseUnavailable         = errors.New("release order storage unavailable")
	ErrReleaseUnknown             = errors.New("release result is pending confirmation")
	ErrReleaseSnapshotUnsupported = errors.New("table contains fields unsupported by the draft snapshot format")
)

type ReleaseOrder = domain.ReleaseOrder
type ReleaseItem = domain.ReleaseItem
type ReleaseField = domain.ReleaseField
type ReleaseFilter = domain.ReleaseFilter

type DraftItemInput struct {
	Operation             string          `json:"operation"`
	ID                    *JSONString     `json:"id"`
	ExpectedRecordVersion string          `json:"expected_record_version"`
	Content               MutationContent `json:"content"`
}

type DraftInput struct {
	TableName       string           `json:"table_name"`
	Items           []DraftItemInput `json:"items"`
	ExpectedVersion string           `json:"expected_version"`
}

// ReleaseOrderSession exposes only control-data writes and consistent baseline
// reads. Preparing a draft cannot call business-row mutation methods.
type ReleaseOrderSession interface {
	PolicySnapshotReader
	ReadRecordBaseline(context.Context, domain.TableSchema, any) (domain.RecordBaseline, error)
	GetReleaseOrder(context.Context, string) (domain.ReleaseOrder, error)
	SaveReleaseOrder(context.Context, domain.ReleaseOrder, bool) error
	BeginReleaseRequest(context.Context, string, string, string, []byte) (*domain.ReleaseOrder, error)
	CompleteReleaseRequest(context.Context, string, string, string, domain.ReleaseOrder) error
	DatabaseTime(context.Context) (time.Time, error)
}

type ReleaseOrderStore interface {
	ExecuteReleaseOrder(context.Context, func(ReleaseOrderSession) error) error
	GetReleaseOrder(context.Context, string) (domain.ReleaseOrder, error)
	ListReleaseOrders(context.Context, domain.ReleaseFilter) ([]domain.ReleaseOrder, error)
}

type ReleaseOrders struct {
	store     ReleaseOrderStore
	snapshots *policySnapshotResolver
}

func NewReleaseOrders(store ReleaseOrderStore) *ReleaseOrders {
	return &ReleaseOrders{store: store, snapshots: newPolicySnapshotResolver(NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())}
}

func (r *ReleaseOrders) Create(ctx context.Context, input DraftInput, key string) (ReleaseOrder, error) {
	actor, err := requireRole(ctx, RoleEditor)
	if err != nil {
		return ReleaseOrder{}, err
	}
	if !roleRequestKey.MatchString(key) {
		return ReleaseOrder{}, ErrReleaseInvalid
	}
	var result ReleaseOrder
	err = r.store.ExecuteReleaseOrder(ctx, func(s ReleaseOrderSession) error {
		digest := releaseDigest(input)
		previous, err := s.BeginReleaseRequest(ctx, actor, "create", key, digest)
		if err != nil {
			return err
		}
		if previous != nil {
			result = *previous
			return nil
		}
		if input.ExpectedVersion != "" {
			return ErrReleaseInvalid
		}
		items, err := r.prepare(ctx, s, input, false)
		if err != nil {
			return err
		}
		now, err := s.DatabaseTime(ctx)
		if err != nil {
			return err
		}
		idBytes := make([]byte, 16)
		if _, err = rand.Read(idBytes); err != nil {
			return ErrReleaseUnavailable
		}
		stamp := now.UTC().Format(time.RFC3339Nano)
		result = ReleaseOrder{ID: hex.EncodeToString(idBytes), TableName: input.TableName, ApplicantID: actor, State: "DRAFT", Version: "1", Items: items, CreatedAt: stamp, UpdatedAt: stamp, History: []domain.ReleaseEvent{{Action: "CREATE", ActorID: actor, At: stamp, Version: "1"}}}
		if err = s.SaveReleaseOrder(ctx, result, true); err != nil {
			return err
		}
		return s.CompleteReleaseRequest(ctx, actor, "create", key, result)
	})
	return result, err
}

func releaseDigest(input any) []byte {
	encoded, _ := json.Marshal(input)
	digest := sha256.Sum256(encoded)
	return digest[:]
}

func (r *ReleaseOrders) Get(ctx context.Context, id string) (ReleaseOrder, error) {
	if _, err := requireRole(ctx, RoleViewer); err != nil {
		return ReleaseOrder{}, err
	}
	return r.store.GetReleaseOrder(ctx, id)
}

func (r *ReleaseOrders) AllowedActions(ctx context.Context, order ReleaseOrder) []string {
	actions := []string{}
	if order.State != "DRAFT" {
		return actions
	}
	actor, err := requireRole(ctx, RoleEditor)
	if err == nil && actor == order.ApplicantID {
		actions = append(actions, "edit", "cancel")
		return actions
	}
	if _, err := requireRole(ctx, RoleAdmin); err == nil {
		actions = append(actions, "cancel")
	}
	return actions
}

func (r *ReleaseOrders) prepare(ctx context.Context, s ReleaseOrderSession, input DraftInput, refreshBaseline bool) ([]ReleaseItem, error) {
	if protectedTable(input.TableName) {
		return nil, ErrProtectedTable
	}
	// T6 (#53) removes this temporary single-item limit.
	if input.TableName == "" || len(input.TableName) > 256 || len(input.Items) != 1 {
		return nil, ErrReleaseInvalid
	}
	snapshot, err := r.snapshots.resolve(ctx, s, input.TableName, mutationPolicySnapshot)
	if err != nil {
		return nil, err
	}

	item := input.Items[0]
	policy, schema := snapshot.mutationPolicy, snapshot.schema
	for _, column := range schema.Columns {
		if column.Type == domain.ColumnTypeUnsupported {
			return nil, ErrReleaseSnapshotUnsupported
		}
	}
	allowed := item.Operation == "ADD" && policy.AllowAdd || item.Operation == "MODIFY" && policy.AllowModify || item.Operation == "DELETE" && policy.AllowDelete
	if !allowed {
		return nil, ErrMutationNotAllowed
	}
	automatic := map[string]bool{}
	reserved := map[string]bool{}
	for _, field := range []*string{policy.CreateOperatorField, policy.CreateTimeField, policy.ModifyOperatorField, policy.ModifyTimeField} {
		if field != nil {
			reserved[*field] = true
			if _, supplied := item.Content[*field]; supplied {
				return nil, ErrInvalidMutation
			}
		}
	}
	if item.Operation == "ADD" {
		for _, field := range []*string{policy.CreateOperatorField, policy.CreateTimeField} {
			if field != nil {
				automatic[*field] = true
			}
		}
	}
	if item.Operation != "DELETE" {
		for _, field := range []*string{policy.ModifyOperatorField, policy.ModifyTimeField} {
			if field != nil {
				automatic[*field] = true
			}
		}
	}

	for _, field := range []*string{policy.CreateOperatorField, policy.ModifyOperatorField} {
		if field != nil && automatic[*field] {
			if err := validateOperatorField(schema, *field); err != nil {
				return nil, err
			}
		}
	}
	if item.Operation == "DELETE" && len(item.Content) > 0 {
		return nil, ErrInvalidMutation
	}
	if item.Operation == "MODIFY" && len(item.Content) == 0 && len(automatic) == 0 {
		return nil, ErrInvalidMutation
	}
	if _, err = mutationValues(schema, item.Content, item.Operation == "ADD"); err != nil {
		return nil, err
	}
	if item.Operation == "ADD" {
		// The known ADD id is supplied once, in content. The server records its identity.
		if item.ID != nil {
			return nil, ErrInvalidMutation
		}
		item.ID = item.Content["id"]
		for _, column := range schema.Columns {
			if _, supplied := item.Content[column.Name]; column.RequiredForInsert() && !supplied && !automatic[column.Name] {
				return nil, ErrMissingRequiredField
			}
		}
	} else if item.ID == nil {
		return nil, ErrInvalidMutation
	}
	baseline := domain.RecordBaseline{}
	if item.ID != nil {
		idColumn, _ := schema.Column("id")
		id, parseErr := domain.ParseColumnValue(idColumn, *item.ID)
		if parseErr != nil {
			return nil, ErrInvalidMutation
		}
		baseline, err = s.ReadRecordBaseline(ctx, schema, id)
		if err != nil {
			return nil, err
		}
	}
	if item.Operation == "ADD" {
		if baseline.Row != nil {
			return nil, ErrDuplicateKey
		}
		if !refreshBaseline && item.ExpectedRecordVersion != "" {
			if err = ValidateRecordVersion(item.ExpectedRecordVersion); err != nil {
				return nil, err
			}
			if item.ExpectedRecordVersion != baseline.Version {
				return nil, ErrRecordVersionConflict
			}
		}
	} else {
		if refreshBaseline {
			item.ExpectedRecordVersion = baseline.Version
		}
		if err = ValidateRecordVersion(item.ExpectedRecordVersion); err != nil {
			return nil, err
		}
		if baseline.Row == nil {
			return nil, ErrMutationRowNotFound
		}
		if baseline.Version != item.ExpectedRecordVersion {
			return nil, ErrRecordVersionConflict
		}
	}
	result := ReleaseItem{Operation: item.Operation, ID: item.ID, ExpectedRecordVersion: baseline.Version, Content: item.Content, Before: baseline.Row, RecordKey: baseline.Key, Fields: []ReleaseField{}}
	if result.Content == nil {
		result.Content = MutationContent{}
	}
	for _, col := range schema.Columns {
		before := baseline.Row[col.Name]
		beforeState := "value"
		if baseline.Row == nil {
			beforeState = "absent"
		} else if before == nil {
			beforeState = "sql_null"
		}
		proposed, supplied := item.Content[col.Name]
		state := "omitted"
		if item.Operation == "DELETE" {
			state = "absent"
		} else if automatic[col.Name] {
			state = "automatic"
		} else if col.Generated {
			state = "generated"
		} else if supplied {
			state = "value"
			if proposed == nil {
				state = "sql_null"
			}
		}
		result.Fields = append(result.Fields, ReleaseField{Name: col.Name, Type: col.Type, Nullable: col.Nullable, Editable: col.Writable() && !reserved[col.Name] && item.Operation != "DELETE" && (col.Name != "id" || item.Operation == "ADD"), BeforeState: beforeState, Before: before, ProposedState: state, Proposed: proposed})
	}

	return []ReleaseItem{result}, nil
}

type CancelReleaseInput struct {
	ExpectedVersion string `json:"expected_version"`
	Reason          string `json:"reason"`
}

func (r *ReleaseOrders) Update(ctx context.Context, id string, input DraftInput, key string) (ReleaseOrder, error) {
	return r.changeDraft(ctx, id, input.ExpectedVersion, "edit", key, input, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		if input.TableName != order.TableName {
			return ErrReleaseInvalid
		}
		for _, item := range input.Items {
			if item.Operation == "ADD" && item.Content["id"] != nil {
				if err := ValidateRecordVersion(item.ExpectedRecordVersion); err != nil {
					return err
				}
			}
		}

		items, err := r.prepare(ctx, s, input, false)
		if err != nil {
			return err
		}
		order.Items = items
		return nil
	})
}
func (r *ReleaseOrders) Cancel(ctx context.Context, id string, input CancelReleaseInput, key string) (ReleaseOrder, error) {
	return r.changeDraft(ctx, id, input.ExpectedVersion, "cancel", key, input, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 {
			return ErrReleaseInvalid
		}
		order.State = "CANCELLED"
		return nil
	})
}
func (r *ReleaseOrders) changeDraft(ctx context.Context, id, version, action, key string, input any, change func(ReleaseOrderSession, *ReleaseOrder) error) (ReleaseOrder, error) {
	actor, err := requireRole(ctx, RoleEditor)
	if err != nil {
		return ReleaseOrder{}, err
	}
	if !roleRequestKey.MatchString(key) {
		return ReleaseOrder{}, ErrReleaseInvalid
	}
	var result ReleaseOrder
	err = r.store.ExecuteReleaseOrder(ctx, func(s ReleaseOrderSession) error {
		// Lock request identity before the order consistently, including retries.
		operation := action + ":" + id
		previous, err := s.BeginReleaseRequest(ctx, actor, operation, key, releaseDigest(input))
		if err != nil {
			return err
		}
		order, err := s.GetReleaseOrder(ctx, id)
		if err != nil {
			return err
		}
		if actor != order.ApplicantID {
			if action != "cancel" {
				return ErrPermissionDenied
			}
			if _, err = requireRole(ctx, RoleAdmin); err != nil {
				return err
			}
		}
		if previous != nil {
			result = *previous
			return nil
		}
		if ValidateRecordVersion(version) != nil || version == "0" {
			return ErrReleaseInvalid
		}
		if order.State != "DRAFT" {
			return ErrReleaseState
		}
		if order.Version != version {
			return ErrReleaseVersionConflict
		}
		next, err := strconv.ParseUint(version, 10, 64)
		if err != nil || next == math.MaxUint64 {
			return ErrReleaseVersionConflict
		}
		if err = change(s, &order); err != nil {
			return err
		}
		now, err := s.DatabaseTime(ctx)
		if err != nil {
			return err
		}
		order.Version = strconv.FormatUint(next+1, 10)
		order.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
		reason := ""
		if cancel, ok := input.(CancelReleaseInput); ok {
			reason = cancel.Reason
		}
		order.History = append(order.History, domain.ReleaseEvent{Action: strings.ToUpper(action), ActorID: actor, At: order.UpdatedAt, Version: order.Version, Reason: reason})
		if err = s.SaveReleaseOrder(ctx, order, false); err != nil {
			return err
		}
		result = order
		return s.CompleteReleaseRequest(ctx, actor, operation, key, result)
	})
	return result, err
}
func (r *ReleaseOrders) List(ctx context.Context, filter ReleaseFilter) ([]ReleaseOrder, error) {
	if _, err := requireRole(ctx, RoleViewer); err != nil {
		return nil, err
	}
	if filter.Limit < 1 || filter.Limit > 100 || len(filter.TableName) > 256 || len(filter.ApplicantID) > 36 || len(filter.ID) > 32 || len(filter.After) > 32 {
		return nil, ErrReleaseInvalid
	}
	switch filter.State {
	case "", "DRAFT", "PENDING_APPROVAL", "APPROVED", "SUCCEEDED", "REJECTED", "CANCELLED", "ROLLED_BACK":
	default:
		return nil, ErrReleaseInvalid
	}
	return r.store.ListReleaseOrders(ctx, filter)
}

// Preview only reads a fresh record baseline for explicit client review. It is
// not an order, a saved request result, a reservation, or permission to publish.
func (r *ReleaseOrders) Preview(ctx context.Context, input DraftInput) ([]ReleaseItem, error) {
	if _, err := requireRole(ctx, RoleEditor); err != nil {
		return nil, err
	}
	var items []ReleaseItem
	err := r.store.ExecuteReleaseOrder(ctx, func(s ReleaseOrderSession) error {
		var err error
		items, err = r.prepare(ctx, s, input, true)
		return err
	})
	return items, err
}
