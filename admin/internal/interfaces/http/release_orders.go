package http

import (
	"errors"
	"strconv"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
)

func registerReleaseOrderRoutes(router *gin.Engine, orders *application.ReleaseOrders) {
	router.POST("/api/v1/release-orders/preview", func(c *gin.Context) {
		var input application.DraftInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		items, err := orders.Preview(c.Request.Context(), input)
		if writeReleaseError(c, err) {
			return
		}
		for i := range items {
			items[i].RecordKey = nil
		}
		c.JSON(200, gin.H{"table_name": input.TableName, "items": items})
	})
	router.GET("/api/v1/release-orders", func(c *gin.Context) {
		limit := 20
		if raw := c.Query("limit"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil {
				writeReleaseError(c, application.ErrReleaseInvalid)
				return
			}
		}
		list, err := orders.List(c.Request.Context(), application.ReleaseFilter{TableName: c.Query("table_name"), ApplicantID: c.Query("applicant_id"), State: c.Query("state"), ID: c.Query("id"), After: c.Query("after"), Limit: limit})
		if writeReleaseError(c, err) {
			return
		}
		response := make([]any, 0, len(list))
		for _, order := range list {
			response = append(response, releaseResponse(order, orders.AllowedActions(c.Request.Context(), order)))
		}
		next := ""
		if len(list) == limit {
			next = list[len(list)-1].ID
		}
		c.JSON(200, gin.H{"orders": response, "next_cursor": next})
	})
	router.PUT("/api/v1/release-orders/:id", func(c *gin.Context) {
		var input application.DraftInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Update(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 200)
	})
	router.POST("/api/v1/release-orders/:id/cancel", func(c *gin.Context) {
		var input application.CancelReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Cancel(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 200)
	})

	router.POST("/api/v1/release-orders", func(c *gin.Context) {
		var input application.DraftInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Create(c.Request.Context(), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 201)
	})
	router.GET("/api/v1/release-orders/:id", func(c *gin.Context) {
		order, err := orders.Get(c.Request.Context(), c.Param("id"))
		if writeReleaseError(c, err) {
			return
		}
		c.JSON(200, releaseResponse(order, orders.AllowedActions(c.Request.Context(), order)))
	})
}
func respondReleaseWrite(c *gin.Context, orders *application.ReleaseOrders, result application.ReleaseOrder, status int) {
	current, err := orders.Get(c.Request.Context(), result.ID)
	if err != nil {
		writeReleaseError(c, application.ErrReleaseUnknown)
		return
	}
	c.JSON(status, releaseResponse(result, orders.AllowedActions(c.Request.Context(), current)))
}
func releaseResponse(order application.ReleaseOrder, actions []string) any {
	// Internal database identity is never a client-supplied or public key.
	for i := range order.Items {
		order.Items[i].RecordKey = nil
	}
	return struct {
		application.ReleaseOrder
		AllowedActions []string `json:"allowed_actions"`
	}{order, actions}
}
func writeReleaseError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	status, code, message := 503, "release_unavailable", "release order storage is unavailable"
	switch {
	case errors.Is(err, application.ErrPermissionDenied):
		status, code, message = 403, "permission_denied", "the current account cannot perform this release action"
	case errors.Is(err, application.ErrSession):
		status, code, message = 401, "authentication_required", "an authenticated account is required"
	case errors.Is(err, application.ErrInvalidMutation):
		status, code, message = 422, "invalid_mutation_content", "mutation content is invalid"
	case errors.Is(err, application.ErrMissingRequiredField):
		status, code, message = 422, "missing_required_field", "required mutation field is missing"

	case errors.Is(err, application.ErrReleaseNotFound):
		status, code, message = 404, "release_not_found", "release order not found"
	case errors.Is(err, application.ErrReleaseInvalid):
		status, code, message = 422, "release_invalid", "invalid release request; exactly one item and a request identifier are required"
	case errors.Is(err, application.ErrReleaseVersionConflict):
		status, code, message = 409, "release_version_conflict", "release order changed; read the latest version and explicitly rebuild"
	case errors.Is(err, application.ErrReleaseIdempotencyConflict):
		status, code, message = 409, "idempotency_conflict", "request identifier was already used with different content"
	case errors.Is(err, application.ErrReleaseState):
		status, code, message = 422, "release_state_invalid", "release order state does not allow this action"
	case errors.Is(err, application.ErrReleaseSnapshotUnsupported):
		status, code, message = 422, "release_snapshot_unsupported", "table contains a field the draft snapshot cannot represent losslessly"
	case errors.Is(err, application.ErrReleaseUnknown):
		status, code, message = 503, "release_result_unknown", "result pending confirmation; retry the original request identifier"
	case errors.Is(err, application.ErrReleaseUnavailable):
	default:
		return writeManagedMutationError(c, err)
	}
	writeError(c, status, code, message)
	return true
}
