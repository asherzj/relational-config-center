package http

import (
	"errors"
	stdhttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/asherzj/relational-config-center/admin/internal/application"
)

func registerReleaseTemplateRoutes(router *gin.Engine, management *application.ReleaseTemplateManagement) {
	router.GET("/api/v1/release-templates", func(c *gin.Context) {
		templates, err := management.List(c.Request.Context())
		if writeReleaseTemplateError(c, err) {
			return
		}
		items := make([]gin.H, 0, len(templates))
		for _, template := range templates {
			items = append(items, releaseTemplateResponse(template))
		}
		c.JSON(stdhttp.StatusOK, gin.H{"templates": items})
	})
	router.GET("/api/v1/release-templates/:code", func(c *gin.Context) {
		template, err := management.Get(c.Request.Context(), c.Param("code"))
		if writeReleaseTemplateError(c, err) {
			return
		}
		c.JSON(stdhttp.StatusOK, releaseTemplateResponse(template))
	})
	router.POST("/api/v1/release-templates", func(c *gin.Context) {
		var body application.PutReleaseTemplate
		if err := decodeRequest(c, &body); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		template, err := management.Create(c.Request.Context(), body, c.GetHeader("Idempotency-Key"))
		if writeReleaseTemplateError(c, err) {
			return
		}
		c.JSON(stdhttp.StatusCreated, releaseTemplateResponse(template))
	})
	router.PUT("/api/v1/release-templates/:code", func(c *gin.Context) {
		var body struct {
			application.PutReleaseTemplate
			ExpectedVersion string `json:"expected_version"`
		}
		if err := decodeRequest(c, &body); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		version, err := strconv.ParseUint(body.ExpectedVersion, 10, 64)
		if err != nil {
			writeReleaseTemplateError(c, application.ErrInvalidReleaseTemplate)
			return
		}
		template, err := management.Replace(c.Request.Context(), c.Param("code"), body.PutReleaseTemplate, version, c.GetHeader("Idempotency-Key"))
		if writeReleaseTemplateError(c, err) {
			return
		}
		c.JSON(stdhttp.StatusOK, releaseTemplateResponse(template))
	})
	for _, operation := range []struct {
		path    string
		enabled bool
	}{{"enable", true}, {"disable", false}} {
		operation := operation
		router.POST("/api/v1/release-templates/:code/"+operation.path, func(c *gin.Context) {
			version, ok := releaseTemplateVersion(c)
			if !ok {
				return
			}
			template, err := management.SetEnabled(c.Request.Context(), c.Param("code"), operation.enabled, version, c.GetHeader("Idempotency-Key"))
			if writeReleaseTemplateError(c, err) {
				return
			}
			c.JSON(stdhttp.StatusOK, releaseTemplateResponse(template))
		})
	}
	router.DELETE("/api/v1/release-templates/:code", func(c *gin.Context) {
		version, ok := releaseTemplateVersion(c)
		if !ok {
			return
		}
		if writeReleaseTemplateError(c, management.Delete(c.Request.Context(), c.Param("code"), version, c.GetHeader("Idempotency-Key"))) {
			return
		}
		c.Status(stdhttp.StatusNoContent)
	})
}

func releaseTemplateVersion(c *gin.Context) (uint64, bool) {
	var body struct {
		ExpectedVersion string `json:"expected_version"`
	}
	if err := decodeRequest(c, &body); err != nil {
		writeRequestDecodeError(c, err)
		return 0, false
	}
	version, err := strconv.ParseUint(body.ExpectedVersion, 10, 64)
	if err != nil {
		writeReleaseTemplateError(c, application.ErrInvalidReleaseTemplate)
		return 0, false
	}
	return version, true
}

func releaseTemplateResponse(template application.ReleaseTemplate) gin.H {
	nodes := make([]gin.H, 0, len(template.Nodes))
	for _, node := range template.Nodes {
		nodes = append(nodes, gin.H{"code": node.Code, "type": node.Type, "name": node.Name, "required_role": node.RequiredRole})
	}
	return gin.H{"code": template.Code, "name": template.Name, "description": template.Description, "type": template.Type, "node_list": nodes, "monitor_list": template.MonitorList, "enabled": template.Enabled, "version": strconv.FormatUint(template.Version, 10), "creator": template.Creator, "modifier": template.Modifier, "created_at": template.CreatedAt, "updated_at": template.UpdatedAt}
}

func writeReleaseTemplateError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, application.ErrPermissionDenied):
		writeError(c, stdhttp.StatusForbidden, "permission_denied", "the current account does not have the required role")
	case errors.Is(err, application.ErrReleaseTemplateInUse):
		writeError(c, stdhttp.StatusConflict, "release_template_in_use", "release template is referenced by a table")
	case errors.Is(err, application.ErrReleaseTemplateNotFound):
		writeError(c, stdhttp.StatusNotFound, "release_template_not_found", "release template not found")
	case errors.Is(err, application.ErrReleaseTemplateExists):
		writeError(c, stdhttp.StatusConflict, "release_template_exists", "release template code already exists")
	case errors.Is(err, application.ErrReleaseTemplateVersionConflict):
		writeError(c, stdhttp.StatusConflict, "release_template_conflict", "release template has changed; review the latest version")
	case errors.Is(err, application.ErrReleaseTemplateIdempotencyConflict):
		writeError(c, stdhttp.StatusConflict, "idempotency_conflict", "request key was already used for different template content")
	case errors.Is(err, application.ErrEmergencyReleaseTemplateProtected):
		writeError(c, stdhttp.StatusConflict, "emergency_template_protected", "emergency release templates must remain enabled and cannot be deleted")
	case errors.Is(err, application.ErrUnknownReleaseType):
		writeError(c, stdhttp.StatusUnprocessableEntity, "invalid_release_type", "release type must be STANDARD or EMERGENCY")
	case errors.Is(err, application.ErrInvalidReleaseTemplateNodes):
		writeError(c, stdhttp.StatusUnprocessableEntity, "invalid_release_template_nodes", "release template nodes do not match the selected release type")
	case errors.Is(err, application.ErrInvalidReleaseTemplate):
		writeError(c, stdhttp.StatusUnprocessableEntity, "invalid_release_template", "release template fields are invalid")
	default:
		writeError(c, stdhttp.StatusServiceUnavailable, "release_template_unavailable", "release template catalog is unavailable")
	}
	return true
}
