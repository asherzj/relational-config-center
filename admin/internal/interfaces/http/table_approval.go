package http

import (
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
)

func registerTableApprovalRoutes(router *gin.Engine, management *application.ApprovalRoleManagement) {
	const path = "/api/v1/table-policies/:table_name/approval-roles"
	router.GET(path, func(c *gin.Context) {
		assignment, err := management.Table(c.Request.Context(), c.Param("table_name"))
		if err != nil {
			writeApprovalRoleError(c, err)
			return
		}
		c.JSON(200, assignment)
	})
	router.PUT(path, func(c *gin.Context) {
		var body struct {
			ExpectedVersion string    `json:"expected_version"`
			RoleIDs         *[]string `json:"role_ids"`
		}
		if decodeRequest(c, &body) != nil || body.RoleIDs == nil {
			writeApprovalRoleError(c, application.ErrApprovalRoleFields)
			return
		}
		assignment, err := management.SaveTable(c.Request.Context(), application.TableApprovalChange{TableName: c.Param("table_name"), ExpectedVersion: body.ExpectedVersion, RoleIDs: *body.RoleIDs, RequestKey: c.GetHeader("Idempotency-Key")})
		if err != nil {
			writeApprovalRoleError(c, err)
			return
		}
		c.JSON(200, assignment)
	})
}
