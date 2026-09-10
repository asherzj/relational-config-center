package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
	"time"
)

type fieldPolicyRecord struct {
	ID                                                   uint64
	Table                                                string `gorm:"column:table_name"`
	FieldName, DisplayName, Description                  string
	DisplayOrder                                         int
	IsVisible, IsQueryable                               bool
	QueryOperators                                       string
	UIType                                               string `gorm:"column:ui_type"`
	UIOptions                                            string `gorm:"column:ui_options"`
	EditableOnAdd, EditableOnModify, IsRequired, Enabled bool
	DefaultValue                                         sql.NullString
	Creator, Modifier                                    string
	CreatedAt, UpdatedAt                                 time.Time
}

func (fieldPolicyRecord) TableName() string { return "rcc_table_field_policies" }
func (a *Adapter) ReadFieldPolicies(ctx context.Context, table string) ([]domain.TableFieldPolicy, error) {
	var records []fieldPolicyRecord
	if err := a.gorm.WithContext(ctx).Where("table_name = ?", table).Order("display_order, field_name").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.TableFieldPolicy, 0, len(records))
	for _, r := range records {
		p := domain.TableFieldPolicy{FieldName: r.FieldName, DisplayName: r.DisplayName, Description: r.Description, DisplayOrder: r.DisplayOrder, IsVisible: r.IsVisible, IsQueryable: r.IsQueryable, UIType: r.UIType, EditableOnAdd: r.EditableOnAdd, EditableOnModify: r.EditableOnModify, IsRequired: r.IsRequired, Enabled: r.Enabled, Creator: r.Creator, Modifier: r.Modifier, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
		if r.QueryOperators == "" || r.QueryOperators == "null" {
			r.QueryOperators = "[]"
		}
		if r.UIOptions == "" || r.UIOptions == "null" {
			r.UIOptions = `{"options":[]}`
		}
		if err := json.Unmarshal([]byte(r.QueryOperators), &p.QueryOperators); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(r.UIOptions), &p.UIOptions); err != nil {
			return nil, err
		}
		if p.UIOptions.Options == nil {
			p.UIOptions.Options = []domain.FieldOption{}
		}
		if r.DefaultValue.Valid {
			p.DefaultValue = json.RawMessage(r.DefaultValue.String)
		}
		result = append(result, p)
	}
	return result, nil
}
func (a *Adapter) ReplaceFieldPolicies(ctx context.Context, table string, policies []domain.TableFieldPolicy, operator string) error {
	return a.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the existing assignment, serializing full replacements even when no
		// field rows exist yet. Every management entry belongs to a real assignment.
		var assignment policyRecord
		if err := tx.Raw("SELECT * FROM rcc_table_policies WHERE table_name = ? FOR UPDATE", table).Scan(&assignment).Error; err != nil {
			return err
		}
		names := make([]string, 0, len(policies))
		for _, p := range policies {
			names = append(names, p.FieldName)
			ops, _ := json.Marshal(p.QueryOperators)
			options, _ := json.Marshal(p.UIOptions)
			var def any
			if p.DefaultValue != nil {
				def = string(p.DefaultValue)
			}
			if err := tx.Exec(`INSERT INTO rcc_table_field_policies (table_name,field_name,display_name,description,display_order,is_visible,is_queryable,query_operators,ui_type,ui_options,editable_on_add,editable_on_modify,is_required,default_value,enabled,creator,modifier) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE display_name=VALUES(display_name),description=VALUES(description),display_order=VALUES(display_order),is_visible=VALUES(is_visible),is_queryable=VALUES(is_queryable),query_operators=VALUES(query_operators),ui_type=VALUES(ui_type),ui_options=VALUES(ui_options),editable_on_add=VALUES(editable_on_add),editable_on_modify=VALUES(editable_on_modify),is_required=VALUES(is_required),default_value=VALUES(default_value),enabled=VALUES(enabled),modifier=VALUES(modifier)`, table, p.FieldName, p.DisplayName, p.Description, p.DisplayOrder, p.IsVisible, p.IsQueryable, string(ops), p.UIType, string(options), p.EditableOnAdd, p.EditableOnModify, p.IsRequired, def, p.Enabled, operator, operator).Error; err != nil {
				return err
			}
		}
		query := tx.Where("table_name = ?", table)
		if len(names) > 0 {
			query = query.Where("field_name NOT IN ?", names)
		}
		return query.Delete(&fieldPolicyRecord{}).Error
	})
}
