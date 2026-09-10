package http

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
	"time"
)

type fieldPolicyDTO struct {
	FieldName        string                      `json:"field_name"`
	DisplayName      string                      `json:"display_name"`
	Description      string                      `json:"description"`
	DisplayOrder     int                         `json:"display_order"`
	IsVisible        bool                        `json:"is_visible"`
	IsQueryable      bool                        `json:"is_queryable"`
	QueryOperators   []application.QueryOperator `json:"query_operators"`
	UIType           string                      `json:"ui_type"`
	UIOptions        application.FieldUIOptions  `json:"ui_options"`
	EditableOnAdd    bool                        `json:"editable_on_add"`
	EditableOnModify bool                        `json:"editable_on_modify"`
	IsRequired       bool                        `json:"is_required"`
	DefaultValue     json.RawMessage             `json:"default_value,omitempty"`
	Enabled          bool                        `json:"enabled"`
}

func (p fieldPolicyDTO) candidate() application.TableFieldPolicy {
	return application.TableFieldPolicy{FieldName: p.FieldName, DisplayName: p.DisplayName, Description: p.Description, DisplayOrder: p.DisplayOrder, IsVisible: p.IsVisible, IsQueryable: p.IsQueryable, QueryOperators: p.QueryOperators, UIType: p.UIType, UIOptions: p.UIOptions, EditableOnAdd: p.EditableOnAdd, EditableOnModify: p.EditableOnModify, IsRequired: p.IsRequired, DefaultValue: p.DefaultValue, Enabled: p.Enabled}
}
func fieldPolicyResponse(p application.TableFieldPolicy) fieldPolicyDTO {
	return fieldPolicyDTO{p.FieldName, p.DisplayName, p.Description, p.DisplayOrder, p.IsVisible, p.IsQueryable, p.QueryOperators, p.UIType, p.UIOptions, p.EditableOnAdd, p.EditableOnModify, p.IsRequired, p.DefaultValue, p.Enabled}
}
func fieldPolicyResultResponse(r application.FieldPolicyResult) gin.H {
	fields := make([]gin.H, 0, len(r.Fields))
	for _, f := range r.Fields {
		var policy any
		var audit any
		if f.Policy != nil {
			policy = fieldPolicyResponse(*f.Policy)
			audit = gin.H{"creator": f.Policy.Creator, "modifier": f.Policy.Modifier, "created_at": f.Policy.CreatedAt.Format(time.RFC3339), "updated_at": f.Policy.UpdatedAt.Format(time.RFC3339)}
		}
		fields = append(fields, gin.H{"field_name": f.Column.Name, "column_type": f.Column.Type, "nullable": f.Column.Nullable, "generated": f.Column.Generated, "auto_increment": f.Column.AutoIncrement, "has_default": f.Column.HasDefault, "policy": policy, "audit": audit, "state": f.State, "warning": f.Warning, "effective": fieldPolicyResponse(f.Effective)})
	}
	return gin.H{"table_name": r.TableName, "fields": fields, "query_capacity": gin.H{"max_conditions": r.QueryCapacity.MaxConditions, "max_values_per_condition": r.QueryCapacity.MaxValuesPerCondition, "queryable_fields": r.QueryCapacity.QueryableFields, "supported": r.QueryCapacity.Supported}}
}
func registerFieldPolicyRoutes(router *gin.Engine, m *application.TableFieldPolicyManagement) {
	router.GET("/api/v1/table-field-policies/:table_name", func(c *gin.Context) {
		r, err := m.Read(c.Request.Context(), c.Param("table_name"))
		if fieldPolicyError(c, err) {
			return
		}
		c.JSON(200, fieldPolicyResultResponse(r))
	})
	router.PUT("/api/v1/table-field-policies/:table_name", func(c *gin.Context) {
		var body struct {
			Policies []fieldPolicyDTO `json:"policies"`
		}
		if err := decodeRequest(c, &body); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		if body.Policies == nil {
			writeError(c, 400, "invalid_field_policy", "policies must be an array")
			return
		}
		policies := make([]application.TableFieldPolicy, 0, len(body.Policies))
		for _, p := range body.Policies {
			policies = append(policies, p.candidate())
		}
		r, err := m.Replace(c.Request.Context(), c.Param("table_name"), policies)
		if fieldPolicyError(c, err) {
			return
		}
		c.JSON(200, fieldPolicyResultResponse(r))
	})
}
func fieldPolicyError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeError(c, 504, "field_policy_timeout", "field configuration timed out; reread to confirm before saving again")
	case errors.Is(err, application.ErrInvalidFieldPolicy):
		var invalid *application.FieldPolicyValidationError
		if errors.As(err, &invalid) {
			c.JSON(422, gin.H{"error": gin.H{"code": "invalid_field_policy", "message": invalid.Reason, "field_name": invalid.FieldName, "request_id": requestID(c)}})
		} else {
			writeError(c, 422, "invalid_field_policy", err.Error())
		}
	case errors.Is(err, application.ErrProtectedTable):
		writeError(c, 403, "protected_table", "protected table")
	case errors.Is(err, application.ErrDatabaseTableNotFound):
		writeError(c, 404, "database_table_not_found", "database table not found")
	case errors.Is(err, application.ErrTablePolicyNotFound):
		writeError(c, 404, "table_policy_not_found", "table policy not found")
	default:
		writeError(c, 503, "field_policy_unavailable", "field configuration could not be read or saved; reread to confirm before saving again")
	}
	return true
}
