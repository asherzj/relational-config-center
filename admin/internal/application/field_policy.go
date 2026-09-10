package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var ErrInvalidFieldPolicy = errors.New("invalid field policy")

type FieldPolicyView struct {
	Column    domain.Column
	Policy    *domain.TableFieldPolicy
	State     string
	Effective domain.TableFieldPolicy
	Warning   string
}
type QueryCapacity struct {
	MaxConditions         int
	MaxValuesPerCondition int
	QueryableFields       int
	Supported             bool
}

type FieldPolicyResult struct {
	QueryCapacity QueryCapacity
	TableName     string
	Fields        []FieldPolicyView
}
type TableFieldPolicyManagement struct {
	metadata    TableMetadataReader
	catalog     domain.TableFieldPolicyCatalog
	assignments domain.TablePolicyCatalog
	mutations   domain.MutationPolicyCatalog
}

func NewTableFieldPolicyManagement(metadata TableMetadataReader, catalog domain.TableFieldPolicyCatalog, assignments domain.TablePolicyCatalog, mutations domain.MutationPolicyCatalog) *TableFieldPolicyManagement {
	return &TableFieldPolicyManagement{metadata, catalog, assignments, mutations}
}
func DefaultFieldPolicy(column domain.Column) domain.TableFieldPolicy {
	return domain.TableFieldPolicy{FieldName: column.Name, DisplayName: column.Name, UIType: "text", QueryOperators: []domain.QueryOperator{domain.QueryOperatorExact}, UIOptions: domain.FieldUIOptions{Options: []domain.FieldOption{}}, IsVisible: true, IsQueryable: true, EditableOnAdd: column.Writable(), EditableOnModify: column.Writable() && column.Name != "id"}
}
func (m *TableFieldPolicyManagement) Read(ctx context.Context, table string) (FieldPolicyResult, error) {
	if protectedTable(table) {
		return FieldPolicyResult{}, ErrProtectedTable
	}
	schema, err := m.metadata.GetTableSchema(ctx, table)
	if err != nil {
		return FieldPolicyResult{}, err
	}
	policies, err := m.catalog.ReadFieldPolicies(ctx, table)
	if err != nil {
		return FieldPolicyResult{}, err
	}
	return fieldPolicyResult(table, schema, policies), nil
}

func fieldPolicyResult(table string, schema domain.TableSchema, policies []domain.TableFieldPolicy) FieldPolicyResult {
	result := FieldPolicyResult{TableName: table, Fields: make([]FieldPolicyView, 0, len(schema.Columns))}
	for _, column := range schema.Columns {
		view := FieldPolicyView{Column: column, State: "missing", Effective: DefaultFieldPolicy(column)}
		for _, policy := range policies {
			if policy.FieldName == column.Name {
				p := policy
				view.Policy = &p
				view.State = "disabled"
				if p.Enabled {
					view.State = "active"
					view.Effective = p
					if err := validateFieldPolicy(p, column); err != nil {
						view.State = "incompatible"
						view.Warning = err.Error()
						view.Effective = DefaultFieldPolicy(column)
					}
				}
				break
			}
		}
		result.Fields = append(result.Fields, view)
	}
	result.QueryCapacity = QueryCapacity{MaxConditions: MaximumQueryConditions, MaxValuesPerCondition: MaximumQueryValues, Supported: true}
	for _, field := range result.Fields {
		if field.Effective.IsQueryable {
			result.QueryCapacity.QueryableFields++
		}
	}
	result.QueryCapacity.Supported = result.QueryCapacity.QueryableFields <= MaximumQueryConditions
	return result
}
func (m *TableFieldPolicyManagement) Replace(ctx context.Context, table string, policies []domain.TableFieldPolicy) (FieldPolicyResult, error) {
	operator, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return FieldPolicyResult{}, err
	}
	if protectedTable(table) {
		return FieldPolicyResult{}, ErrProtectedTable
	}
	schema, err := m.metadata.GetTableSchema(ctx, table)
	if err != nil {
		return FieldPolicyResult{}, err
	}
	assignment, err := m.assignments.Get(ctx, table)
	if err != nil {
		return FieldPolicyResult{}, err
	}
	mutation, err := m.mutations.GetMutationPolicy(ctx, assignment.MutationPolicyCode)
	if err != nil {
		return FieldPolicyResult{}, err
	}
	if len(policies) > 1024 {
		return FieldPolicyResult{}, invalidField(table, "字段规则最多1024项")
	}
	existing, err := m.catalog.ReadFieldPolicies(ctx, table)
	if err != nil {
		return FieldPolicyResult{}, err
	}
	seen := map[string]bool{}
	for i := range policies {
		p := &policies[i]
		column, found := schema.Column(p.FieldName)
		if !found || seen[p.FieldName] {
			return FieldPolicyResult{}, invalidField(p.FieldName, "字段不存在或重复")
		}
		seen[p.FieldName] = true
		if p.UIType == "" {
			p.UIType = "text"
		}
		if p.QueryOperators == nil {
			p.QueryOperators = []domain.QueryOperator{}
		}
		if p.UIOptions.Options == nil {
			p.UIOptions.Options = []domain.FieldOption{}
		}
		unchangedDisabled := false
		if !p.Enabled {
			for _, old := range existing {
				if old.FieldName == p.FieldName && sameFieldPolicyConfiguration(*p, old) {
					unchangedDisabled = true
					break
				}
			}
		}
		if unchangedDisabled {
			continue
		}
		if err := validateFieldPolicy(*p, column); err != nil {
			return FieldPolicyResult{}, err
		}
		autofilled := false
		for _, name := range []*string{mutation.CreateOperatorField, mutation.CreateTimeField, mutation.ModifyOperatorField, mutation.ModifyTimeField} {
			if name != nil && *name == column.Name {
				autofilled = true
			}
		}
		if p.Enabled && !p.EditableOnAdd && column.RequiredForInsert() && !autofilled {
			return FieldPolicyResult{}, invalidField(column.Name, "数据库必填且无默认值或自动填写来源，不能关闭新增编辑")
		}
	}
	capacity := fieldPolicyResult(table, schema, policies).QueryCapacity
	if !capacity.Supported {
		return FieldPolicyResult{}, invalidField(table, fmt.Sprintf("真实可查询字段共%d个，超过平台上限%d；请关闭部分字段的查询", capacity.QueryableFields, capacity.MaxConditions))
	}
	if err = m.catalog.ReplaceFieldPolicies(ctx, table, policies, operator); err != nil {
		return FieldPolicyResult{}, err
	}
	return m.Read(ctx, table)
}

// Disabling an unchanged persisted rule remains possible after Schema drift.
// Editing or reenabling it always passes full validation again.
func sameFieldPolicyConfiguration(candidate, stored domain.TableFieldPolicy) bool {
	candidate.Enabled = stored.Enabled
	candidate.Creator = stored.Creator
	candidate.Modifier = stored.Modifier
	candidate.CreatedAt = stored.CreatedAt
	candidate.UpdatedAt = stored.UpdatedAt
	return reflect.DeepEqual(candidate, stored)
}
