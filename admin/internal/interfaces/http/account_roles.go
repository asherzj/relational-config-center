package http

import (
	"context"
	"errors"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

func registerAccountRoleRoutes(router *gin.Engine, management *application.AccountRoleManagement) {
	router.GET("/api/v1/account-roles/:id/history", func(c *gin.Context) {
		var before uint64
		if cursor := c.Query("before"); cursor != "" {
			var err error
			before, err = strconv.ParseUint(cursor, 10, 64)
			if err != nil {
				writeRoleError(c, application.ErrAccountFields)
				return
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		events, err := management.History(ctx, c.Param("id"), before, 50)
		if err != nil {
			writeRoleError(c, err)
			return
		}
		rows := make([]gin.H, 0, len(events))
		for _, event := range events {
			rows = append(rows, gin.H{"id": strconv.FormatUint(event.ID, 10), "actor_kind": event.ActorKind, "actor_id": event.ActorID, "account_id": event.AccountID, "before_roles": event.BeforeRoles.Names(), "after_roles": event.AfterRoles.Names(), "version": strconv.FormatUint(event.Version, 10), "created_at": event.CreatedAt})
		}
		next := ""
		if len(events) == 50 {
			next = strconv.FormatUint(events[len(events)-1].ID, 10)
		}
		c.JSON(200, gin.H{"events": rows, "next_cursor": next})
	})
	router.GET("/api/v1/account-roles", func(c *gin.Context) {
		limit := 25
		if value := c.Query("limit"); value != "" {
			var err error
			limit, err = strconv.Atoi(value)
			if err != nil {
				writeRoleError(c, application.ErrAccountFields)
				return
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		accounts, err := management.List(ctx, c.Query("q"), c.Query("after"), limit)
		if err != nil {
			writeRoleError(c, err)
			return
		}
		rows := make([]gin.H, 0, len(accounts))
		for _, account := range accounts {
			rows = append(rows, roleAccountResponse(account))
		}
		next := ""
		if len(accounts) == limit {
			next = accounts[len(accounts)-1].ID
		}
		c.JSON(200, gin.H{"accounts": rows, "next_cursor": next})
	})
	router.PUT("/api/v1/account-roles/:id", func(c *gin.Context) {
		var body struct {
			Roles           []string `json:"roles"`
			ExpectedVersion string   `json:"expected_version"`
		}
		if err := decodeRequest(c, &body); err != nil {
			writeRoleError(c, application.ErrAccountFields)
			return
		}
		version, err := strconv.ParseUint(body.ExpectedVersion, 10, 64)
		if err != nil {
			writeRoleError(c, application.ErrAccountFields)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		result, err := management.Change(ctx, c.Param("id"), body.Roles, version, c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeRoleError(c, err)
			return
		}
		c.JSON(200, roleAccountResponse(result))
	})
}

func roleAccountResponse(account application.RoleAccount) gin.H {
	return gin.H{"id": account.ID, "username": account.Username, "display_name": account.DisplayName, "enabled": account.Enabled, "roles": account.Roles.Names(), "version": strconv.FormatUint(account.RoleVersion, 10)}
}

func writeRoleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrLastAdministrator):
		writeError(c, 409, "last_administrator", "cannot remove the last enabled administrator")
	case errors.Is(err, application.ErrRoleIdempotencyConflict):
		writeError(c, 409, "idempotency_conflict", "request key was already used for different role content")
	case errors.Is(err, application.ErrPermissionDenied):
		writeError(c, 403, "permission_denied", "the current account does not have the required role")
	case errors.Is(err, application.ErrInvalidRoles):
		writeError(c, 422, "invalid_account_roles", "select one or more distinct supported roles")
	case errors.Is(err, application.ErrRoleVersionConflict):
		writeError(c, 409, "account_roles_conflict", "account roles have changed; review the latest roles")
	case errors.Is(err, application.ErrAccountNotFound):
		writeError(c, 404, "account_not_found", "account not found")
	default:
		writeAuthError(c, err)
	}
}
