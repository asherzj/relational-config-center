package http

import (
	"context"
	"errors"
	"strconv"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
)

func registerReleaseOrderRoutes(router *gin.Engine, orders *application.ReleaseOrders) {
	router.POST("/api/v1/release-orders/:id/reprepare", func(c *gin.Context) {
		var input application.CopyReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Reprepare(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 201)
	})
	router.GET("/api/v1/release-orders/:id/people", func(c *gin.Context) {
		people, err := orders.People(c.Request.Context(), c.Param("id"))
		if writeReleaseError(c, err) {
			return
		}
		c.JSON(200, gin.H{"people": people})
	})
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
			items[i].ConcurrencyKeys = nil
			items[i].RecordKey = nil
			items[i].RecordTable = ""
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
			summary := struct {
				application.ReleaseOrderSummary
				AllowedActions []string `json:"allowed_actions"`
			}{order, orders.AllowedActions(c.Request.Context(), application.ReleaseOrder{ID: order.ID, State: order.State, ApplicantID: order.ApplicantID, RollbackOfID: order.RollbackOfID, RollbackOrderID: order.RollbackOrderID, RollbackPending: order.RollbackPending})}
			response = append(response, summary)
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
	for action, decide := range map[string]func(context.Context, string, application.ReleaseDecisionInput, string) (application.ReleaseOrder, error){"approve": orders.Approve, "reject": orders.Reject} {
		router.POST("/api/v1/release-orders/:id/"+action, func(c *gin.Context) {
			var input application.ReleaseDecisionInput
			if err := decodeRequest(c, &input); err != nil {
				writeRequestDecodeError(c, err)
				return
			}
			order, err := decide(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
			if writeReleaseError(c, err) {
				return
			}
			respondReleaseWrite(c, orders, order, 200)
		})
	}
	router.POST("/api/v1/release-orders/:id/quick-rollback", func(c *gin.Context) {
		var input application.QuickRollbackInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		result, err := orders.QuickRollback(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, result, 200)
	})
	router.POST("/api/v1/release-orders/:id/quick-rollback/preview", func(c *gin.Context) {
		var input application.SubmitReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		preview, err := orders.PreviewQuickRollback(c.Request.Context(), c.Param("id"), input)
		if writeReleaseError(c, err) {
			return
		}
		for i := range preview.Items {
			preview.Items[i].ConcurrencyKeys = nil
			preview.Items[i].RecordKey = nil
			preview.Items[i].RecordTable = ""
		}
		c.JSON(200, preview)
	})
	router.POST("/api/v1/release-orders/:id/rollback", func(c *gin.Context) {
		var input application.CancelReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Rollback(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 201)
	})
	router.POST("/api/v1/release-orders/:id/copy", func(c *gin.Context) {
		var input application.CopyReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Copy(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 201)
	})
	router.POST("/api/v1/release-orders/:id/complete", func(c *gin.Context) {
		var input application.SubmitReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Complete(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 200)
	})
	router.POST("/api/v1/release-orders/:id/execute", func(c *gin.Context) {
		var input application.SubmitReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Execute(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
		if writeReleaseError(c, err) {
			return
		}
		respondReleaseWrite(c, orders, order, 200)
	})
	router.POST("/api/v1/release-orders/:id/submit", func(c *gin.Context) {
		var input application.SubmitReleaseInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		order, err := orders.Submit(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
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
	// Internal execution metadata and database identity are persisted, not client input.
	order.Frozen = nil
	for i := range order.Items {
		order.Items[i].ConcurrencyKeys = nil
		order.Items[i].RecordKey = nil
		order.Items[i].RecordTable = ""
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
	var itemError *application.ReleaseItemError
	if errors.As(err, &itemError) {
		c.Set("release_item_index", itemError.Index)
	}
	var conflict *application.ReleaseTargetConflict
	if errors.As(err, &conflict) {
		c.Set("release_target_conflict", conflict)
	}
	status, code, message := 503, "release_unavailable", "release order storage is unavailable"
	switch {
	case errors.Is(err, application.ErrRollbackConflict):
		status, code, message = 409, "rollback_conflict", "the original publication already has an active rollback request"
	case errors.Is(err, application.ErrRollbackLocked):
		status, code, message = 422, "rollback_locked", "rollback intent is fixed to the original result; cancel or reject it and reapply from the original order"
	case errors.Is(err, application.ErrRollbackRestoreMismatch):
		status, code, message = 422, "rollback_restore_mismatch", "current schema, rules or database effects cannot restore every original business value"
	case errors.Is(err, application.ErrPermissionDenied):
		status, code, message = 403, "permission_denied", "the current account cannot perform this release action"
	case errors.Is(err, application.ErrSession):
		status, code, message = 401, "authentication_required", "an authenticated account is required"
	case errors.Is(err, application.ErrConcurrencyKeyValue):
		status, code, message = 422, "concurrency_key_value_required", "supply determinable values for every concurrency key field"
	case errors.Is(err, application.ErrConcurrencyKeyInvalid):
		status, code, message = 422, "concurrency_key_invalid", "concurrency key is incompatible with current fields or automatic values"
	case errors.Is(err, application.ErrInvalidMutation):
		status, code, message = 422, "invalid_mutation_content", "mutation content is invalid"
	case errors.Is(err, application.ErrMissingRequiredField):
		status, code, message = 422, "missing_required_field", "required mutation field is missing"

	case errors.Is(err, application.ErrReleaseFrozenChanged):
		status, code, message = 409, "release_frozen_changed", "approved execution semantics changed; cancel and rebuild the proposal"
	case errors.Is(err, application.ErrPublicationUnsupported):
		status, code, message = 422, "publication_unsupported", "publication cannot track all fields, identities or implicit writes for this table"
	case errors.Is(err, application.ErrPublicationMetadataPermission):
		status, code, message = 422, "publication_metadata_permission", "deployment requires PROCESS to verify the complete InnoDB foreign-key dictionary"
	case errors.Is(err, application.ErrReleaseNotFound):
		status, code, message = 404, "release_not_found", "release order not found"
	case errors.Is(err, application.ErrReleaseCrossTable):
		status, code, message = 422, "release_cross_table", "all items must belong to the order table"
	case errors.Is(err, application.ErrReleaseItemLimit):
		status, code, message = 422, "release_item_limit", "a release order must contain 1 to 1000 items"
	case errors.Is(err, application.ErrReleaseResultLimit):
		status, code, message = 422, "release_result_limit", "the complete release document and result must fit the 8 MiB encoded JSON budget"
	case errors.Is(err, application.ErrReleaseFieldLimit):
		status, code, message = 422, "release_field_limit", "each submitted field must fit the 64 KiB UTF-8 limit"
	case errors.Is(err, application.ErrReleaseDuplicateTarget):
		status, code, message = 422, "release_duplicate_target", "a known record identity appears more than once in this order"
	case errors.Is(err, application.ErrReleaseTitle):
		status, code, message = 422, "release_title_invalid", "release title must contain 1 to 100 characters"
	case errors.Is(err, application.ErrReleaseInvalid):
		status, code, message = 422, "release_invalid", "release request content, version, reason or identifier is invalid"
	case errors.Is(err, application.ErrReleaseAutoIDAmbiguous):
		status, code, message = 422, "release_auto_id_ambiguous", "zero would generate an auto-increment id in the current database mode; omit id instead"
	case errors.Is(err, application.ErrReleaseTargetConflict):
		status, code, message = 409, "release_target_conflict", "a target is reserved by an unfinished release order"
	case errors.Is(err, application.ErrReleaseVersionConflict):
		status, code, message = 409, "release_version_conflict", "release order changed; read the latest version and explicitly rebuild"
	case errors.Is(err, application.ErrReleaseIdempotencyConflict):
		status, code, message = 409, "idempotency_conflict", "request identifier was already used with different content"
	case errors.Is(err, application.ErrReleaseState):
		status, code, message = 422, "release_state_invalid", "release order state does not allow this action"
	case errors.Is(err, application.ErrReleaseMetadataPermission):
		status, code, message = 422, "release_metadata_permission", "deployment requires an explicit TRIGGER metadata grant to verify execution semantics"
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
