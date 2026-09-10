package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

const ReleaseFieldBytes = 64 << 10
const ReleaseResultBytes = 8 << 20
const ReleaseContinuationHeadroom = 64 << 10
const ReleaseTransportHeadroom = 1024

var (
	ErrReleaseTitle               = errors.New("release title must contain 1 to 100 characters")
	ErrReleaseCrossTable          = errors.New("release item belongs to another table")
	ErrReleaseItemLimit           = errors.New("release must contain 1 to 1000 items")
	ErrReleaseResultLimit         = errors.New("release result exceeds 8 MiB")
	ErrReleaseFieldLimit          = errors.New("release field exceeds 64 KiB")
	ErrReleaseDuplicateTarget     = errors.New("known record identity appears more than once")
	ErrReleaseAutoIDAmbiguous     = errors.New("zero auto-increment id would generate a new identity; omit id instead")
	ErrReleaseTargetConflict      = errors.New("release target is reserved by another order")
	ErrReleaseNotFound            = errors.New("release order not found")
	ErrReleaseInvalid             = errors.New("invalid release order")
	ErrReleaseVersionConflict     = errors.New("release order version conflict")
	ErrReleaseState               = errors.New("release order state does not allow this action")
	ErrReleaseIdempotencyConflict = errors.New("release request identifier already used with different content")
	ErrReleaseUnavailable         = errors.New("release order storage unavailable")
	ErrReleaseUnknown             = errors.New("release commit outcome is unknown")
	ErrReleaseMetadataPermission  = errors.New("explicit TRIGGER metadata permission is required")
	ErrReleaseSnapshotUnsupported = errors.New("table contains fields unsupported by the draft snapshot format")
)

// ReleaseItemError locates an error in the original zero-based request order.
type ReleaseItemError struct {
	Index int
	Cause error
}

func (e *ReleaseItemError) Error() string { return e.Cause.Error() }
func (e *ReleaseItemError) Unwrap() error { return e.Cause }

type ReleaseOrderSummary = domain.ReleaseOrderSummary
type ReleaseOrder = domain.ReleaseOrder
type ReleaseItem = domain.ReleaseItem
type ReleaseField = domain.ReleaseField
type ReleaseFilter = domain.ReleaseFilter

type DraftItemInput struct {
	DetailID              string          `json:"detail_id,omitempty"`
	TableName             string          `json:"table_name,omitempty"`
	Operation             string          `json:"operation"`
	ID                    *JSONString     `json:"id"`
	ExpectedRecordVersion string          `json:"expected_record_version"`
	Content               MutationContent `json:"content"`
}

type DraftInput struct {
	Changes         *DraftChanges    `json:"changes,omitempty"`
	Title           string           `json:"title"`
	TableName       string           `json:"table_name"`
	Items           []DraftItemInput `json:"items"`
	ExpectedVersion string           `json:"expected_version"`
}

// ReleaseOrderSession exposes only control-data writes and consistent baseline
// reads. Preparing a draft cannot call business-row mutation methods.
type ReleaseOrderSession interface {
	AppendReleaseFailure(context.Context, string, domain.ReleaseEvent) error
	PolicySnapshotReader
	ResolveReleaseTable(context.Context, string) (string, error)
	ReadRecordBaselines(context.Context, domain.TableSchema, []any) ([]domain.RecordBaseline, error)
	ReadRollbackBaselines(context.Context, domain.TableSchema, []any, domain.ReleaseOrder, []string) ([]domain.RecordBaseline, error)
	LockAndReadTableExecutionSchema(context.Context, string) (domain.TableExecutionSchema, error)
	ReserveReleaseTargets(context.Context, string, []domain.ActiveTarget) error
	ReplaceReleaseTargets(context.Context, string, []domain.ActiveTarget) error
	ReadConcurrencyKeys(context.Context, domain.TableSchema, []string, []domain.Row) ([][]byte, error)
	ReplaceReleaseTableReferences(context.Context, string, []string) error
	ReleaseTargets(context.Context, string) error
	GetReleaseOrder(context.Context, string) (domain.ReleaseOrder, error)
	SaveReleaseOrder(context.Context, domain.ReleaseOrder, bool) error
	BeginReleaseRequest(context.Context, string, string, string, []byte) (*domain.ReleaseOrder, error)
	CompleteReleaseRequest(context.Context, string, string, string, domain.ReleaseOrder) error
	DatabaseTime(context.Context) (time.Time, error)
}

type ReleaseOrderStore interface {
	ExecuteReleaseOrder(context.Context, func(ReleaseOrderSession) error) error
	ExecutePublication(context.Context, func(PublicationSession) error) error
	GetReleaseOrder(context.Context, string) (domain.ReleaseOrder, error)
	ListReleaseOrders(context.Context, domain.ReleaseFilter) ([]domain.ReleaseOrderSummary, error)
	AccountDisplayNames(context.Context, []string) (map[string]string, error)
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
		if input.ExpectedVersion != "" || input.Changes != nil {
			return ErrReleaseInvalid
		}
		if err := validateReleaseTitle(input.Title); err != nil {
			return err
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
		result = ReleaseOrder{Title: input.Title, ID: hex.EncodeToString(idBytes), TableName: input.TableName, ApplicantID: actor, State: "DRAFT", Version: "1", Items: items, CreatedAt: stamp, UpdatedAt: stamp, History: []domain.ReleaseEvent{{Action: "CREATE", ActorID: actor, At: stamp, Version: "1"}}}
		result.TableNames = releaseTableNames(result.Items)
		if err = s.SaveReleaseOrder(ctx, result, true); err != nil {
			return err
		}
		if err := replaceDraftTargets(ctx, s, result); err != nil {
			return err
		}
		return s.CompleteReleaseRequest(ctx, actor, "create", key, result)
	})
	return result, r.describeTargetConflict(ctx, err)
}

func validateReleaseTitle(title string) error {
	if !utf8.ValidString(title) || strings.TrimSpace(title) == "" || utf8.RuneCountInString(title) > 100 {
		return ErrReleaseTitle
	}
	return nil
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

// People resolves only identities already visible in this order. It does not
// expose account search or role-management data to ordinary viewers.
func (r *ReleaseOrders) People(ctx context.Context, id string) (map[string]string, error) {
	order, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{order.ApplicantID: true}
	for _, event := range order.History {
		seen[event.ActorID] = true
	}
	if order.Publication != nil {
		seen[order.Publication.PublisherID] = true
	}
	if order.Rollback != nil {
		seen[order.Rollback.PublisherID] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return r.store.AccountDisplayNames(ctx, ids)
}

func (r *ReleaseOrders) AllowedActions(ctx context.Context, order ReleaseOrder) []string {
	actions := []string{}
	for _, action := range []string{"edit", "submit", "approve", "reject", "cancel", "copy", "execute", "rollback", "complete", "quick-rollback", "reprepare"} {
		if releaseOrderActionState(order, action) && authorizeReleaseAction(ctx, order, action) == nil {
			actions = append(actions, action)
		}
	}
	return actions
}
func releaseActionRole(action string) AccountRoles {
	if action == "execute" || action == "complete" || action == "quick-rollback" {
		return RolePublisher
	}
	if action == "approve" || action == "reject" {
		return RoleApprover
	}
	return RoleEditor
}
func authorizeReleaseAction(ctx context.Context, order ReleaseOrder, action string) error {
	actor, err := requireRole(ctx, releaseActionRole(action))
	if err != nil {
		return err
	}
	if action == "approve" || action == "reject" {
		if actor == order.ApplicantID {
			return ErrPermissionDenied
		}
		return nil
	}
	if action == "execute" || action == "complete" || action == "quick-rollback" || action == "copy" || action == "rollback" || actor == order.ApplicantID {
		return nil
	}
	if action == "cancel" || action == "reprepare" {
		_, err := requireRole(ctx, RoleAdmin)
		return err
	}
	return ErrPermissionDenied
}
func releaseOrderActionState(order ReleaseOrder, action string) bool {
	if order.RollbackOfID != "" && (action == "edit" || action == "copy" || action == "complete" || action == "quick-rollback" || action == "rollback" || action == "reprepare") {
		return false
	}
	if action == "rollback" && order.RollbackPending {
		return false
	}
	return releaseActionState(order.State, action)
}

func releaseActionState(state, action string) bool {
	switch action {
	case "reprepare":
		return state == "APPROVED"
	case "complete", "quick-rollback":
		return state == "SUCCEEDED"
	case "rollback":
		return false
	case "execute":
		return state == "APPROVED"
	case "copy":
		return state == "REJECTED" || state == "CANCELLED"
	case "edit", "submit":
		return state == "DRAFT"
	case "approve", "reject":
		return state == "PENDING_APPROVAL"
	case "cancel":
		return state == "DRAFT" || state == "PENDING_APPROVAL" || state == "APPROVED"
	}
	return false
}

func (r *ReleaseOrders) prepare(ctx context.Context, s ReleaseOrderSession, input DraftInput, refreshBaseline bool) ([]ReleaseItem, error) {
	return r.prepareInput(ctx, s, input, refreshBaseline, nil)
}

func (r *ReleaseOrders) prepareInput(ctx context.Context, s ReleaseOrderSession, input DraftInput, refreshBaseline bool, source *ReleaseOrder) ([]ReleaseItem, error) {
	if len(input.Items) > 1000 {
		return nil, ErrReleaseItemLimit
	}
	// The aggregate input table remains a shorthand for explicitly single-table
	// requests. Persisted details always carry their own authoritative identity.
	groups := map[string][]int{}
	physical := map[string]string{}
	for index := range input.Items {
		item := &input.Items[index]
		if item.TableName == "" {
			item.TableName = input.TableName
		}
		if item.TableName == "" || len(item.TableName) > 256 {
			return nil, &ReleaseItemError{Index: index, Cause: ErrReleaseInvalid}
		}
		if protectedTable(item.TableName) {
			return nil, &ReleaseItemError{Index: index, Cause: ErrProtectedTable}
		}
		name, known := physical[item.TableName]
		if !known {
			var err error
			name, err = s.ResolveReleaseTable(ctx, item.TableName)
			if err != nil {
				return nil, &ReleaseItemError{Index: index, Cause: err}
			}
			physical[item.TableName] = name
		}
		item.TableName = name
		groups[item.TableName] = append(groups[item.TableName], index)
	}
	// Every table guard precedes the first catalog or business snapshot read.
	// Waiting for B after snapshot A exists would otherwise retain stale B rows.
	tables := sortedTableKeys(groups)
	for _, table := range tables {
		if err := guardDraftReleaseTable(ctx, s, table); err != nil {
			return nil, RemapReleaseItemError(err, groups[table])
		}
	}
	items := make([]ReleaseItem, len(input.Items))
	detailIDs := map[string]bool{}
	for _, table := range tables {
		positions := groups[table]
		snapshot, err := r.snapshots.resolve(ctx, s, table, mutationPolicySnapshot)
		if err != nil {
			return nil, RemapReleaseItemError(err, positions)
		}
		ids := make([]any, len(positions))
		stableIDs := make([]string, len(positions))
		idColumn, _ := snapshot.schema.Column("id")
		for local, index := range positions {
			item := input.Items[index]
			stableIDs[local] = item.DetailID
			for name := range item.Content {
				if len(name) > 256 {
					return nil, &ReleaseItemError{Index: index, Cause: ErrInvalidMutation}
				}
			}
			id := item.ID
			if item.Operation == "ADD" {
				id = item.Content["id"]
			}
			if id != nil {
				ids[local], err = domain.ParseColumnValue(idColumn, *id)
				if err != nil {
					return nil, &ReleaseItemError{Index: index, Cause: ErrInvalidMutation}
				}
			}
		}
		var baselines []domain.RecordBaseline
		if source == nil {
			baselines, err = s.ReadRecordBaselines(ctx, snapshot.schema, ids)
		} else {
			baselines, err = s.ReadRollbackBaselines(ctx, snapshot.schema, ids, *source, stableIDs)
		}
		if err != nil {
			return nil, RemapReleaseItemError(err, positions)
		}
		groupItems := make([]ReleaseItem, len(positions))
		seen := map[string]bool{}
		for local, index := range positions {
			item := input.Items[index]
			if item.DetailID == "" {
				item.DetailID, err = newDetailID()
				if err != nil {
					return nil, err
				}
			}
			if decoded, err := hex.DecodeString(item.DetailID); err != nil || len(decoded) != 16 || detailIDs[item.DetailID] {
				return nil, &ReleaseItemError{Index: index, Cause: ErrReleaseInvalid}
			}
			detailIDs[item.DetailID] = true
			prepared, err := prepareReleaseItem(snapshot.schema, snapshot.mutationPolicy, item, baselines[local], refreshBaseline, source != nil)
			if err != nil {
				return nil, &ReleaseItemError{Index: index, Cause: err}
			}
			key := string(prepared[0].RecordKey)
			if key != "" {
				if seen[key] {
					return nil, &ReleaseItemError{Index: index, Cause: ErrReleaseDuplicateTarget}
				}
				seen[key] = true
			}
			groupItems[local] = prepared[0]
		}
		if err := prepareConcurrencyTargets(ctx, s, snapshot, groupItems); err != nil {
			return nil, RemapReleaseItemError(err, positions)
		}
		for local, index := range positions {
			items[index] = groupItems[local]
		}
	}
	return items, nil
}

func prepareReleaseItem(schema domain.TableSchema, policy domain.MutationPolicy, item DraftItemInput, baseline domain.RecordBaseline, refreshBaseline, historical bool) ([]ReleaseItem, error) {
	var err error
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
	if _, err = releaseMutationValues(schema, item.Content, item.Operation == "ADD", historical); err != nil {
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
		idColumn, _ := schema.Column("id")
		if item.ID == nil && !idColumn.AutoIncrement {
			// LAST_INSERT_ID identifies only auto-increment inserts. A default
			// expression on another primary key cannot supply a trusted identity.
			return nil, ErrPublicationUnsupported
		}
	} else if item.ID == nil {
		return nil, ErrInvalidMutation
	}
	if item.Operation == "ADD" {
		if baseline.GeneratesIDOnInsert {
			return nil, ErrReleaseAutoIDAmbiguous
		}
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
	result := ReleaseItem{DetailID: item.DetailID, TableName: schema.Name, Operation: item.Operation, ID: item.ID, ExpectedRecordVersion: baseline.Version, Content: item.Content, Before: baseline.Row, RecordKey: baseline.Key, RecordTable: baseline.TableName, Fields: []ReleaseField{}}
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
	return r.changeOrder(ctx, id, input.ExpectedVersion, "edit", key, input, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		if err := validateReleaseTitle(input.Title); err != nil {
			return err
		}
		if input.Changes != nil {
			if input.Items != nil {
				return ErrReleaseInvalid
			}
			items, err := r.editDraftDetails(ctx, s, *order, *input.Changes, input.TableName)
			if err != nil {
				return err
			}
			order.Title = input.Title
			order.Items = items
			return replaceDraftTargets(ctx, s, *order)
		}
		for index, item := range input.Items {
			if item.Operation == "ADD" && item.Content["id"] != nil {
				if err := ValidateRecordVersion(item.ExpectedRecordVersion); err != nil {
					return &ReleaseItemError{Index: index, Cause: err}
				}
			}
		}

		items, err := r.prepare(ctx, s, input, false)
		if err != nil {
			return err
		}
		order.Title = input.Title
		order.Items = items
		return replaceDraftTargets(ctx, s, *order)
	})
}

type SubmitReleaseInput struct {
	ExpectedVersion string `json:"expected_version"`
}

func (r *ReleaseOrders) Submit(ctx context.Context, id string, input SubmitReleaseInput, key string) (ReleaseOrder, error) {
	return r.changeOrder(ctx, id, input.ExpectedVersion, "submit", key, input, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		if len(order.Items) == 0 {
			return ErrReleaseItemLimit
		}
		tables, err := r.resolveReleaseTables(ctx, s, order.Items, false)
		if err != nil {
			return err
		}
		items, err := r.prepareOrder(ctx, s, *order)
		if err != nil {
			return err
		}
		for index, item := range items {
			// A maintenance write may change values without advancing the platform
			// version; DDL may change equality weights without changing values.
			// Only an explicit save may replace the acknowledged baseline/targets.
			saved := order.Items[index]
			if !bytes.Equal(releaseDigest(item.Before), releaseDigest(saved.Before)) ||
				item.RecordTable != saved.RecordTable || !bytes.Equal(item.RecordKey, saved.RecordKey) ||
				!slices.EqualFunc(item.ConcurrencyKeys, saved.ConcurrencyKeys, bytes.Equal) {
				return &ReleaseItemError{Index: index, Cause: ErrRecordVersionConflict}
			}
		}
		order.Items = items
		order.FrozenTables = frozenReleaseTables(tables)
		// Temporary display alias; execution always resolves the detail's table.
		first := order.FrozenTables[items[0].TableName]
		order.Frozen = &first
		order.FrozenDigest = frozenOrderDigest(*order)
		targets := []domain.ActiveTarget{}
		for index, item := range items {
			if len(item.RecordKey) > 0 {
				targets = append(targets, domain.ActiveTarget{ItemIndex: index, TableName: item.RecordTable, RecordKey: item.RecordKey})
			}
		}
		if err := s.ReserveReleaseTargets(ctx, order.ID, targets); err != nil {
			return err
		}
		order.State = "PENDING_APPROVAL"
		return nil
	})
}

// Complete ends a successful ordinary publication without changing its data or
// claiming downstream delivery. The workflow and target release commit together.
func (r *ReleaseOrders) Complete(ctx context.Context, id string, input SubmitReleaseInput, key string) (ReleaseOrder, error) {
	return r.changeOrder(ctx, id, input.ExpectedVersion, "complete", key, input, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		order.State = "COMPLETED"
		return s.ReleaseTargets(ctx, order.ID)
	})
}

func (r *ReleaseOrders) Cancel(ctx context.Context, id string, input CancelReleaseInput, key string) (ReleaseOrder, error) {
	return r.changeOrder(ctx, id, input.ExpectedVersion, "cancel", key, input, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 {
			return ErrReleaseInvalid
		}
		order.State = "CANCELLED"
		if err := r.finishRollback(ctx, s, *order, false); err != nil {
			return err
		}
		return s.ReleaseTargets(ctx, order.ID)
	})
}

// An approval records a current independent decision; later role changes do not
// rewrite that history. Publication checks its own current actor in T5.
type ReleaseDecisionInput = CancelReleaseInput

func (r *ReleaseOrders) Approve(ctx context.Context, id string, input ReleaseDecisionInput, key string) (ReleaseOrder, error) {
	return r.decide(ctx, id, "approve", input, key)
}
func (r *ReleaseOrders) Reject(ctx context.Context, id string, input ReleaseDecisionInput, key string) (ReleaseOrder, error) {
	return r.decide(ctx, id, "reject", input, key)
}
func (r *ReleaseOrders) decide(ctx context.Context, id, action string, input ReleaseDecisionInput, key string) (ReleaseOrder, error) {
	return r.changeOrder(ctx, id, input.ExpectedVersion, action, key, input, func(s ReleaseOrderSession, order *ReleaseOrder) error {
		if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 {
			return ErrReleaseInvalid
		}
		order.State = "APPROVED"
		if action == "reject" {
			order.State = "REJECTED"
			if err := r.finishRollback(ctx, s, *order, false); err != nil {
				return err
			}
			return s.ReleaseTargets(ctx, order.ID)
		}
		return nil
	})
}

func (r *ReleaseOrders) changeOrder(ctx context.Context, id, version, action, key string, input any, change func(ReleaseOrderSession, *ReleaseOrder) error) (ReleaseOrder, error) {
	return r.changeOrderUsing(ctx, id, version, action, key, input, r.store.ExecuteReleaseOrder, change)
}

func (r *ReleaseOrders) changeOrderUsing(ctx context.Context, id, version, action, key string, input any, execute func(context.Context, func(ReleaseOrderSession) error) error, change func(ReleaseOrderSession, *ReleaseOrder) error) (ReleaseOrder, error) {
	actor, err := requireRole(ctx, releaseActionRole(action))
	if err != nil {
		return ReleaseOrder{}, err
	}
	if !roleRequestKey.MatchString(key) {
		return ReleaseOrder{}, ErrReleaseInvalid
	}
	hint, err := r.store.GetReleaseOrder(ctx, id)
	if err != nil {
		return ReleaseOrder{}, err
	}
	var result ReleaseOrder
	err = execute(ctx, func(s ReleaseOrderSession) error {
		// Lock request identity before the order consistently, including retries.
		operation := action + ":" + id
		previous, err := s.BeginReleaseRequest(ctx, actor, operation, key, releaseDigest(input))
		if err != nil {
			return err
		}
		// Every reverse action locks its immutable original first. Discovery was
		// outside this transaction so it cannot start a stale RR snapshot.
		if hint.RollbackOfID != "" {
			if _, err := s.GetReleaseOrder(ctx, hint.RollbackOfID); err != nil {
				return err
			}
		}
		order, err := s.GetReleaseOrder(ctx, id)
		if err != nil {
			return err
		}
		if err := authorizeReleaseAction(ctx, order, action); err != nil {
			return err
		}
		if previous != nil {
			result = *previous
			return nil
		}
		if ValidateRecordVersion(version) != nil || version == "0" {
			return ErrReleaseInvalid
		}
		if order.Version != version {
			return ErrReleaseVersionConflict
		}
		if order.RollbackOfID != "" && (action == "edit" || action == "copy" || action == "reprepare") {
			return ErrRollbackLocked
		}
		if !releaseOrderActionState(order, action) {
			return ErrReleaseState
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
		order.TableNames = releaseTableNames(order.Items)
		if err = s.SaveReleaseOrder(ctx, order, false); err != nil {
			return err
		}
		result = order
		return s.CompleteReleaseRequest(ctx, actor, operation, key, result)
	})
	return result, r.describeTargetConflict(ctx, err)
}
func (r *ReleaseOrders) List(ctx context.Context, filter ReleaseFilter) ([]domain.ReleaseOrderSummary, error) {
	if _, err := requireRole(ctx, RoleViewer); err != nil {
		return nil, err
	}
	if filter.Limit < 1 || filter.Limit > 100 || len(filter.TableName) > 256 || len(filter.ApplicantID) > 36 || len(filter.ID) > 32 || len(filter.After) > 32 {
		return nil, ErrReleaseInvalid
	}
	switch filter.State {
	case "", "DRAFT", "PENDING_APPROVAL", "APPROVED", "SUCCEEDED", "COMPLETED", "REJECTED", "CANCELLED", "ROLLED_BACK":
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

// Copy preserves the rejected/cancelled source and creates an independent draft.
// The caller must explicitly confirm every newly reviewed record baseline.
type CopyReleaseInput struct {
	ExpectedVersion string           `json:"expected_version"`
	Confirmed       bool             `json:"confirmed"`
	Items           []DraftItemInput `json:"items"`
}

func (r *ReleaseOrders) Copy(ctx context.Context, id string, input CopyReleaseInput, key string) (ReleaseOrder, error) {
	return r.copyOrder(ctx, id, input, key, false)
}

// Reprepare atomically retires one approved ordinary order and creates a fresh
// editable draft from the caller-confirmed current record baselines. Approval
// never crosses the boundary between the two orders.
func (r *ReleaseOrders) Reprepare(ctx context.Context, id string, input CopyReleaseInput, key string) (ReleaseOrder, error) {
	return r.copyOrder(ctx, id, input, key, true)
}

func (r *ReleaseOrders) copyOrder(ctx context.Context, id string, input CopyReleaseInput, key string, reprepare bool) (ReleaseOrder, error) {
	actor, err := requireRole(ctx, RoleEditor)
	if err != nil {
		return ReleaseOrder{}, err
	}
	if !roleRequestKey.MatchString(key) {
		return ReleaseOrder{}, ErrReleaseInvalid
	}
	var result ReleaseOrder
	err = r.store.ExecuteReleaseOrder(ctx, func(s ReleaseOrderSession) error {
		action := "copy"
		if reprepare {
			action = "reprepare"
		}
		operation := action + ":" + id
		previous, err := s.BeginReleaseRequest(ctx, actor, operation, key, releaseDigest(input))
		if err != nil {
			return err
		}
		source, err := s.GetReleaseOrder(ctx, id)
		if err != nil {
			return err
		}
		if err := authorizeReleaseAction(ctx, source, action); err != nil {
			return err
		}
		if previous != nil {
			result = *previous
			return nil
		}
		if !input.Confirmed || ValidateRecordVersion(input.ExpectedVersion) != nil || input.ExpectedVersion == "0" {
			return ErrReleaseInvalid
		}
		if source.Version != input.ExpectedVersion {
			return ErrReleaseVersionConflict
		}
		if source.RollbackOfID != "" {
			return ErrRollbackLocked
		}
		if !releaseOrderActionState(source, action) {
			return ErrReleaseState
		}
		if len(input.Items) != len(source.Items) {
			return ErrReleaseInvalid
		}
		for i, entry := range input.Items {
			saved := source.Items[i]
			if entry.DetailID != "" && entry.DetailID != saved.DetailID {
				return &ReleaseItemError{Index: i, Cause: ErrReleaseInvalid}
			}
			if entry.TableName != "" && entry.TableName != saved.TableName {
				return &ReleaseItemError{Index: i, Cause: ErrReleaseCrossTable}
			}
			// A copy confirmation refreshes record baselines only. The source's
			// stable detail and table identities remain authoritative, including
			// for older callers that omitted these now-explicit fields.
			entry.DetailID, entry.TableName = saved.DetailID, saved.TableName
			input.Items[i] = entry
			expected := DraftItemInput{DetailID: entry.DetailID, TableName: entry.TableName, Operation: saved.Operation, ID: saved.ID, Content: saved.Content}
			if expected.Operation == "ADD" {
				expected.ID = nil
			}
			candidate := entry
			candidate.ExpectedRecordVersion = ""
			if !bytes.Equal(releaseDigest(candidate), releaseDigest(expected)) {
				return &ReleaseItemError{Index: i, Cause: ErrReleaseInvalid}
			}
			if saved.ID != nil && ValidateRecordVersion(entry.ExpectedRecordVersion) != nil {
				return &ReleaseItemError{Index: i, Cause: ErrReleaseInvalid}
			}
		}
		items, err := r.prepare(ctx, s, DraftInput{TableName: source.TableName, Items: input.Items}, false)
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
		historyAction := "COPY"
		historyReason := "复制"
		if reprepare {
			historyAction = "REPREPARE"
			historyReason = "重新准备"
		}
		result = ReleaseOrder{Title: source.Title, ID: hex.EncodeToString(idBytes), CopiedFromID: source.ID, TableName: source.TableName, ApplicantID: actor, State: "DRAFT", Version: "1", Items: items, CreatedAt: stamp, UpdatedAt: stamp, History: []domain.ReleaseEvent{{Action: historyAction, ActorID: actor, At: stamp, Version: "1", RelatedOrderID: source.ID}}}
		if reprepare {
			source.State = "CANCELLED"
		}
		if err := appendRelatedReleaseEvent(&source, actor, stamp, historyAction, historyReason, result.ID); err != nil {
			return err
		}
		if err := s.SaveReleaseOrder(ctx, source, false); err != nil {
			return err
		}
		if reprepare {
			if err := s.ReleaseTargets(ctx, source.ID); err != nil {
				return err
			}
		}
		result.TableNames = releaseTableNames(result.Items)
		if err := s.SaveReleaseOrder(ctx, result, true); err != nil {
			return err
		}
		if err := replaceDraftTargets(ctx, s, result); err != nil {
			return err
		}
		return s.CompleteReleaseRequest(ctx, actor, operation, key, result)
	})
	return result, r.describeTargetConflict(ctx, err)
}
