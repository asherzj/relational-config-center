package http

import (
	"errors"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

func registerTableReleaseTemplateRoutes(router *gin.Engine, m *application.TableReleaseTemplateManagement) {
	router.GET("/api/v1/table-policies/:table_name/release-templates", func(c *gin.Context) {
		rows, err := m.List(c.Request.Context(), c.Param("table_name"))
		if writeTableReleaseTemplateError(c, err) {
			return
		}
		result := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			result = append(result, tableReleaseTemplateResponse(row))
		}
		c.JSON(http.StatusOK, gin.H{"associations": result})
	})
	router.PUT("/api/v1/table-policies/:table_name/release-templates/:release_type", func(c *gin.Context) {
		var body struct {
			TemplateCode    string `json:"template_code"`
			Enabled         *bool  `json:"enabled"`
			ExpectedVersion string `json:"expected_version"`
		}
		if err := decodeRequest(c, &body); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		version, err := strconv.ParseUint(body.ExpectedVersion, 10, 64)
		if err != nil || body.Enabled == nil {
			writeTableReleaseTemplateError(c, application.ErrInvalidTableReleaseTemplate)
			return
		}
		row, err := m.Put(c.Request.Context(), c.Param("table_name"), c.Param("release_type"), body.TemplateCode, *body.Enabled, version, c.GetHeader("Idempotency-Key"))
		if writeTableReleaseTemplateError(c, err) {
			return
		}
		c.JSON(http.StatusOK, tableReleaseTemplateResponse(row))
	})
}

func tableReleaseTemplateResponse(r application.TableReleaseTemplate) gin.H {
	return gin.H{"table_name": r.TableName, "type": r.Type, "template_code": r.TemplateCode, "template_name": r.TemplateName, "template_enabled": r.TemplateEnabled, "enabled": r.Enabled, "version": strconv.FormatUint(r.Version, 10), "creator": r.Creator, "modifier": r.Modifier, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt}
}
func writeTableReleaseTemplateError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, application.ErrTablePolicyNotFound):
		writeError(c, 404, "table_policy_not_found", "table policy not found")
	case errors.Is(err, application.ErrProtectedTable):
		writeError(c, 403, "protected_table", "control tables cannot manage themselves")
	case errors.Is(err, application.ErrInvalidTableReleaseTemplate):
		writeError(c, 422, "invalid_table_release_template", "choose an enabled template of the same release type")
	case errors.Is(err, application.ErrTableReleaseTemplateConflict):
		writeError(c, 409, "table_release_template_conflict", "association has changed; review the latest version")
	case errors.Is(err, application.ErrEmergencyAssociationProtected):
		writeError(c, 409, "emergency_association_protected", "emergency association must remain enabled")
	default:
		return writeReleaseTemplateError(c, err)
	}
	return true
}
