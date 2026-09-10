package http

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
)

func registerApprovalRoleRoutes(router *gin.Engine, management *application.ApprovalRoleManagement) {
	group := router.Group("/api/v1/approval-roles")
	group.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	group.GET("", func(c *gin.Context) {
		limit := 25
		if value := c.Query("limit"); value != "" {
			var err error
			limit, err = strconv.Atoi(value)
			if err != nil {
				writeApprovalRoleError(c, application.ErrApprovalRoleFields)
				return
			}
		}
		roles, err := management.List(c.Request.Context(), c.Query("q"), c.Query("after"), limit)
		if err != nil {
			writeApprovalRoleError(c, err)
			return
		}
		rows := make([]gin.H, 0, len(roles))
		for _, role := range roles {
			rows = append(rows, approvalRoleResponse(role))
		}
		next := ""
		if len(roles) == limit {
			next = roles[len(roles)-1].ID
		}
		c.JSON(200, gin.H{"roles": rows, "next_cursor": next})
	})
	group.GET("/:id", func(c *gin.Context) {
		role, err := management.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeApprovalRoleError(c, err)
			return
		}
		c.JSON(200, approvalRoleResponse(role))
	})
	save := func(c *gin.Context) {
		var body struct {
			Name            string    `json:"name"`
			Description     string    `json:"description"`
			Enabled         *bool     `json:"enabled"`
			MemberIDs       *[]string `json:"member_ids"`
			ExpectedVersion string    `json:"expected_version"`
		}
		if decodeRequest(c, &body) != nil || body.Enabled == nil || body.MemberIDs == nil {
			writeApprovalRoleError(c, application.ErrApprovalRoleFields)
			return
		}
		var version uint64
		if c.Param("id") != "" {
			var err error
			version, err = strconv.ParseUint(body.ExpectedVersion, 10, 64)
			if err != nil {
				writeApprovalRoleError(c, application.ErrApprovalRoleFields)
				return
			}
		} else if body.ExpectedVersion != "" {
			writeApprovalRoleError(c, application.ErrApprovalRoleFields)
			return
		}
		role, err := management.Save(c.Request.Context(), application.ApprovalRoleChange{ID: c.Param("id"), Name: body.Name, Description: body.Description, Enabled: *body.Enabled, MemberIDs: *body.MemberIDs, ExpectedVersion: version, RequestKey: c.GetHeader("Idempotency-Key")})
		if err != nil {
			writeApprovalRoleError(c, err)
			return
		}
		status := 200
		if c.Param("id") == "" {
			status = 201
		}
		c.JSON(status, approvalRoleResponse(role))
	}
	group.POST("", save)
	group.PUT("/:id", save)
	group.DELETE("/:id", func(c *gin.Context) {
		var body struct {
			ExpectedVersion string `json:"expected_version"`
		}
		if decodeRequest(c, &body) != nil {
			writeApprovalRoleError(c, application.ErrApprovalRoleFields)
			return
		}
		version, err := strconv.ParseUint(body.ExpectedVersion, 10, 64)
		if err != nil {
			writeApprovalRoleError(c, application.ErrApprovalRoleFields)
			return
		}
		role, err := management.Delete(c.Request.Context(), c.Param("id"), version, c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeApprovalRoleError(c, err)
			return
		}
		c.JSON(200, approvalRoleResponse(role))
	})
}
func approvalRoleResponse(role application.ApprovalRole) gin.H {
	members := make([]gin.H, 0, len(role.Members))
	for _, member := range role.Members {
		members = append(members, gin.H{"id": member.ID, "username": member.Username, "display_name": member.DisplayName, "enabled": member.Enabled})
	}
	return gin.H{"id": role.ID, "name": role.Name, "description": role.Description, "enabled": role.Enabled, "version": strconv.FormatUint(role.Version, 10), "members": members, "referenced": role.Referenced, "deleted": role.Deleted, "creator": role.Creator, "modifier": role.Modifier, "created_at": role.CreatedAt, "updated_at": role.UpdatedAt}
}
func writeApprovalRoleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrApprovalRoleNotSaved):
		writeError(c, 503, "approval_role_not_saved", "this attempt was not committed; the content can be corrected and saved again")
	case errors.Is(err, application.ErrApprovalRoleFields):
		writeError(c, 422, "invalid_approval_role", "name, members, version or request key is invalid")
	case errors.Is(err, application.ErrApprovalRoleNotFound):
		writeError(c, 404, "approval_role_not_found", "approval role not found")
	case errors.Is(err, application.ErrApprovalRoleReferenced):
		writeError(c, 409, "approval_role_referenced", "a previously referenced approval role cannot be deleted")
	case errors.Is(err, application.ErrApprovalRoleVersion):
		writeError(c, 409, "approval_role_conflict", "approval role has changed; review the latest version")
	default:
		writeRoleError(c, err)
	}
}
