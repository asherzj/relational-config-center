package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

const SingleTableMutationPolicyType = "single_table_mutation"

var (
	ErrInvalidMutationPolicyDefinition = errors.New("invalid Mutation Policy definition")
	ErrUnknownMutationPolicyType       = errors.New("unknown Mutation Policy Type")
	ErrInvalidMutationPolicyRules      = errors.New("invalid Mutation Policy rules")
	ErrMutationPolicyNotAssignable     = errors.New("Mutation Policy is not assignable")
)

type MutationOperation string

const (
	MutationOperationAdd    MutationOperation = "ADD"
	MutationOperationModify MutationOperation = "MODIFY"
	MutationOperationDelete MutationOperation = "DELETE"
)

type MutationPolicyType struct {
	Code       string
	Operations []MutationOperation
}

// MutationPolicyTypeRegistry is the immutable, code-owned list of executable
// mutation contracts. Every entry covers all three operations; allow_* on the
// concrete Policy remains the sole authorization source.
type MutationPolicyTypeRegistry struct {
	types map[string]MutationPolicyType
}

func NewMutationPolicyTypeRegistry() *MutationPolicyTypeRegistry {
	operations := []MutationOperation{MutationOperationAdd, MutationOperationModify, MutationOperationDelete}
	return &MutationPolicyTypeRegistry{types: map[string]MutationPolicyType{
		SingleTableMutationPolicyType: {Code: SingleTableMutationPolicyType, Operations: operations},
	}}
}

func (registry *MutationPolicyTypeRegistry) List() []MutationPolicyType {
	registered := registry.types[SingleTableMutationPolicyType]
	registered.Operations = append([]MutationOperation(nil), registered.Operations...)
	return []MutationPolicyType{registered}
}

func (registry *MutationPolicyTypeRegistry) Validate(policy domain.MutationPolicy) error {
	registered, found := registry.types[policy.TypeCode]
	if !found {
		return fmt.Errorf("%w: %s", ErrUnknownMutationPolicyType, policy.TypeCode)
	}
	if !implementsAllMutationOperations(registered.Operations) {
		return fmt.Errorf("%w: registered Type does not implement ADD, MODIFY, and DELETE", ErrUnknownMutationPolicyType)
	}
	return registry.validatePersistentRules(policy)
}

func (*MutationPolicyTypeRegistry) validatePersistentRules(policy domain.MutationPolicy) error {
	targets := []*string{
		policy.CreateOperatorField,
		policy.CreateTimeField,
		policy.ModifyOperatorField,
		policy.ModifyTimeField,
	}
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target == nil {
			continue
		}
		if strings.EqualFold(*target, "id") {
			return fmt.Errorf("%w: Auto Fill target id is the primary key", ErrInvalidMutationPolicyRules)
		}
		if !fieldNamePattern.MatchString(*target) {
			return fmt.Errorf("%w: unsafe Auto Fill target", ErrInvalidMutationPolicyRules)
		}
		normalizedTarget := strings.ToLower(*target)
		if _, duplicated := seen[normalizedTarget]; duplicated {
			return fmt.Errorf("%w: duplicated Auto Fill target", ErrInvalidMutationPolicyRules)
		}
		seen[normalizedTarget] = struct{}{}
	}
	if !policy.AllowAdd && (policy.CreateOperatorField != nil || policy.CreateTimeField != nil) {
		return fmt.Errorf("%w: Create Auto Fill requires ADD", ErrInvalidMutationPolicyRules)
	}
	if !policy.AllowAdd && !policy.AllowModify && (policy.ModifyOperatorField != nil || policy.ModifyTimeField != nil) {
		return fmt.Errorf("%w: Modify Auto Fill requires ADD or MODIFY", ErrInvalidMutationPolicyRules)
	}
	return nil
}

func (registry *MutationPolicyTypeRegistry) ValidateForTable(policy domain.MutationPolicy, schema domain.TableSchema) error {
	if err := registry.Validate(policy); err != nil {
		return err
	}
	checks := []struct {
		field *string
		time  bool
	}{
		{field: policy.CreateOperatorField},
		{field: policy.CreateTimeField, time: true},
		{field: policy.ModifyOperatorField},
		{field: policy.ModifyTimeField, time: true},
	}
	for _, check := range checks {
		if check.field == nil {
			continue
		}
		column, found := schema.Column(*check.field)
		if !found || !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
			return fmt.Errorf("%w: Auto Fill target %s is not a writable live column", ErrInvalidMutationPolicyRules, *check.field)
		}
		if check.time {
			switch column.Type {
			case domain.ColumnTypeDate, domain.ColumnTypeTime, domain.ColumnTypeDateTime, domain.ColumnTypeTimestamp:
			default:
				return fmt.Errorf("%w: Auto Fill time target %s is not temporal", ErrInvalidMutationPolicyRules, *check.field)
			}
		} else if column.Type != domain.ColumnTypeString {
			return fmt.Errorf("%w: Auto Fill operator target %s is not textual", ErrInvalidMutationPolicyRules, *check.field)
		}
	}
	return nil
}

func implementsAllMutationOperations(operations []MutationOperation) bool {
	implemented := make(map[MutationOperation]struct{}, len(operations))
	for _, operation := range operations {
		implemented[operation] = struct{}{}
	}
	for _, required := range []MutationOperation{MutationOperationAdd, MutationOperationModify, MutationOperationDelete} {
		if _, found := implemented[required]; !found {
			return false
		}
	}
	return true
}

type PutMutationPolicy struct {
	Code                string
	Name                string
	Description         string
	TypeCode            string
	AllowAdd            bool
	AllowModify         bool
	AllowDelete         bool
	CreateOperatorField *string
	CreateTimeField     *string
	ModifyOperatorField *string
	ModifyTimeField     *string
}

type MutationPolicyManagement struct {
	catalog  domain.MutationPolicyCatalog
	registry *MutationPolicyTypeRegistry
}

func NewMutationPolicyManagement(catalog domain.MutationPolicyCatalog, registry *MutationPolicyTypeRegistry) *MutationPolicyManagement {
	return &MutationPolicyManagement{catalog: catalog, registry: registry}
}

func (management *MutationPolicyManagement) Types() []MutationPolicyType {
	return management.registry.List()
}

func (management *MutationPolicyManagement) Create(ctx context.Context, candidate PutMutationPolicy) (domain.MutationPolicy, error) {
	operator, identityErr := requireRole(ctx, RoleAdmin)
	if identityErr != nil {
		return domain.MutationPolicy{}, identityErr
	}
	policy, err := draftMutationPolicy(candidate)
	if err != nil {
		return domain.MutationPolicy{}, err
	}
	if err := management.registry.validatePersistentRules(policy); err != nil {
		return domain.MutationPolicy{}, err
	}
	return management.catalog.CreateMutationPolicy(ctx, policy, operator)
}

func (management *MutationPolicyManagement) List(ctx context.Context) ([]domain.MutationPolicy, error) {
	return management.catalog.ListMutationPolicies(ctx)
}

func (management *MutationPolicyManagement) Get(ctx context.Context, code string) (domain.MutationPolicy, error) {
	return management.catalog.GetMutationPolicy(ctx, code)
}

func (management *MutationPolicyManagement) GetForNewAssignment(ctx context.Context, code string) (domain.MutationPolicy, error) {
	policy, err := management.catalog.GetMutationPolicy(ctx, code)
	if err != nil {
		return domain.MutationPolicy{}, err
	}
	if policy.Status != domain.PolicyStatusActive {
		return domain.MutationPolicy{}, ErrMutationPolicyNotAssignable
	}
	return policy, nil
}

// ValidateForTable verifies fixed Auto Fill slots against the live Schema.
// Operator slots accept string columns; Time slots accept temporal columns.
func (management *MutationPolicyManagement) ValidateForTable(policy domain.MutationPolicy, schema domain.TableSchema) error {
	return management.registry.ValidateForTable(policy, schema)
}

func (management *MutationPolicyManagement) ReplaceDraft(ctx context.Context, code string, candidate PutMutationPolicy) (domain.MutationPolicy, error) {
	operator, identityErr := requireRole(ctx, RoleAdmin)
	if identityErr != nil {
		return domain.MutationPolicy{}, identityErr
	}
	if candidate.Code != code {
		return domain.MutationPolicy{}, ErrInvalidPolicyCode
	}
	policy, err := draftMutationPolicy(candidate)
	if err != nil {
		return domain.MutationPolicy{}, err
	}
	if err := management.registry.validatePersistentRules(policy); err != nil {
		return domain.MutationPolicy{}, err
	}
	return management.catalog.ReplaceDraftMutationPolicy(ctx, policy, operator)
}

func (management *MutationPolicyManagement) Activate(ctx context.Context, code string) (domain.MutationPolicy, error) {
	operator, identityErr := requireRole(ctx, RoleAdmin)
	if identityErr != nil {
		return domain.MutationPolicy{}, identityErr
	}
	policy, err := management.catalog.GetMutationPolicy(ctx, code)
	if err != nil {
		return domain.MutationPolicy{}, err
	}
	if policy.Status != domain.PolicyStatusDraft {
		return domain.MutationPolicy{}, ErrInvalidPolicyTransition
	}
	if err := management.registry.Validate(policy); err != nil {
		return domain.MutationPolicy{}, err
	}
	return management.catalog.SetMutationPolicyStatus(ctx, code, domain.PolicyStatusDraft, domain.PolicyStatusActive, operator)
}

func (management *MutationPolicyManagement) Deprecate(ctx context.Context, code string) (domain.MutationPolicy, error) {
	operator, identityErr := requireRole(ctx, RoleAdmin)
	if identityErr != nil {
		return domain.MutationPolicy{}, identityErr
	}
	policy, err := management.catalog.GetMutationPolicy(ctx, code)
	if err != nil {
		return domain.MutationPolicy{}, err
	}
	if policy.Status != domain.PolicyStatusActive {
		return domain.MutationPolicy{}, ErrInvalidPolicyTransition
	}
	return management.catalog.SetMutationPolicyStatus(ctx, code, domain.PolicyStatusActive, domain.PolicyStatusDeprecated, operator)
}

func (management *MutationPolicyManagement) UpdateMetadata(ctx context.Context, code, name, description string) (domain.MutationPolicy, error) {
	operator, identityErr := requireRole(ctx, RoleAdmin)
	if identityErr != nil {
		return domain.MutationPolicy{}, identityErr
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 500 {
		return domain.MutationPolicy{}, ErrInvalidMutationPolicyDefinition
	}
	policy, err := management.catalog.GetMutationPolicy(ctx, code)
	if err != nil {
		return domain.MutationPolicy{}, err
	}
	if policy.Status != domain.PolicyStatusActive && policy.Status != domain.PolicyStatusDeprecated {
		return domain.MutationPolicy{}, ErrInvalidPolicyTransition
	}
	return management.catalog.UpdateMutationPolicyMetadata(ctx, code, name, description, operator)
}

func (management *MutationPolicyManagement) DeleteDraft(ctx context.Context, code string) error {
	if _, identityErr := requireRole(ctx, RoleAdmin); identityErr != nil {
		return identityErr
	}
	policy, err := management.catalog.GetMutationPolicy(ctx, code)
	if err != nil {
		return err
	}
	if policy.Status != domain.PolicyStatusDraft {
		return ErrInvalidPolicyTransition
	}
	return management.catalog.DeleteDraftMutationPolicy(ctx, code)
}

func draftMutationPolicy(candidate PutMutationPolicy) (domain.MutationPolicy, error) {
	code := strings.TrimSpace(candidate.Code)
	if !validPolicyCode(code) {
		return domain.MutationPolicy{}, ErrInvalidPolicyCode
	}
	name := strings.TrimSpace(candidate.Name)
	description := strings.TrimSpace(candidate.Description)
	typeCode := strings.TrimSpace(candidate.TypeCode)
	if name == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 500 || typeCode == "" || len(typeCode) > 64 {
		return domain.MutationPolicy{}, ErrInvalidMutationPolicyDefinition
	}
	createOperatorField := normalizedOptionalField(candidate.CreateOperatorField)
	createTimeField := normalizedOptionalField(candidate.CreateTimeField)
	modifyOperatorField := normalizedOptionalField(candidate.ModifyOperatorField)
	modifyTimeField := normalizedOptionalField(candidate.ModifyTimeField)
	for _, field := range []*string{createOperatorField, createTimeField, modifyOperatorField, modifyTimeField} {
		if field != nil && len(*field) > 64 {
			return domain.MutationPolicy{}, ErrInvalidMutationPolicyDefinition
		}
	}
	return domain.MutationPolicy{
		Code: code, Name: name, Description: description, TypeCode: typeCode,
		AllowAdd: candidate.AllowAdd, AllowModify: candidate.AllowModify, AllowDelete: candidate.AllowDelete,
		CreateOperatorField: createOperatorField,
		CreateTimeField:     createTimeField,
		ModifyOperatorField: modifyOperatorField,
		ModifyTimeField:     modifyTimeField,
		Status:              domain.PolicyStatusDraft,
	}, nil
}

func normalizedOptionalField(field *string) *string {
	if field == nil {
		return nil
	}
	value := strings.TrimSpace(*field)
	if value == "" {
		return nil
	}
	return &value
}
