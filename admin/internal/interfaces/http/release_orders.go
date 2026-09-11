package http

import (
	"context"
	"errors"
	"strconv"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
)

func registerReleaseOrderRoutes(router *gin.Engine, orders *application.ReleaseOrders) {
	router.POST("/api/v1/release-orders/:id/notification-read", func(c *gin.Context) {
		var input struct {
			Sequence string `json:"sequence"`
		}
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		result, err := orders.AcknowledgeNotification(c.Request.Context(), c.Param("id"), input.Sequence)
		if writeReleaseError(c, err) {
			return
		}
		c.JSON(200, result)
	})
	router.GET("/api/v1/approval-notifications", func(c *gin.Context) {
		counts, err := orders.NotificationCounts(c.Request.Context())
		if writeReleaseError(c, err) {
			return
		}
		c.JSON(200, counts)
	})
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
		c.JSON(200, gin.H{"items": items})
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
		if raw, ok := c.GetQuery("unread"); ok && (raw != "true" && raw != "false" || !c.Request.URL.Query().Has("view")) {
			writeReleaseError(c, application.ErrReleaseInvalid)
			return
		}
		filter := application.ReleaseFilter{UnreadOnly: c.Query("unread") == "true", TableName: c.Query("table_name"), ApplicantID: c.Query("applicant_id"), State: c.Query("state"), ID: c.Query("id"), After: c.Query("after"), Limit: limit}
		var list []application.ReleaseOrderSummary
		var err error
		next := ""
		if c.Request.URL.Query().Has("view") {
			list, next, err = orders.NotificationOrders(c.Request.Context(), c.Query("view"), filter)
		} else {
			list, err = orders.List(c.Request.Context(), filter)
			if len(list) == limit && len(list) > 0 {
				next = list[len(list)-1].ID
			}
		}
		if writeReleaseError(c, err) {
			return
		}
		response := make([]any, 0, len(list))
		for _, order := range list {
			if order.RollbackTableFlows == nil {
				order.RollbackTableFlows = []application.ReleaseTableFlow{}
			}
			summary := struct {
				application.ReleaseOrderSummary
				AllowedActions []string `json:"allowed_actions"`
			}{order, orders.AllowedActions(c.Request.Context(), application.ReleaseOrder{RollbackTableFlows: order.RollbackTableFlows, EmergencyReason: order.EmergencyReason, ReleaseType: order.ReleaseType, TableFlows: order.TableFlows, MissingFlowTables: order.MissingFlowTables, ID: order.ID, State: order.State, ApplicantID: order.ApplicantID, TableNames: order.TableNames, Approvals: order.Approvals, ApprovalContext: order.ApprovalContext})}
			response = append(response, summary)
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
	router.POST("/api/v1/release-orders/:id/rollback-reason", func(c *gin.Context) {
		var input application.RollbackReasonInput
		if err := decodeRequest(c, &input); err != nil {
			writeRequestDecodeError(c, err)
			return
		}
		result, err := orders.ChangeRollbackReason(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
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
		preview, err := orders.PreviewQuickRollback(c.Request.Context(), c.Param("id"), input, c.GetHeader("Idempotency-Key"))
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
		var input application.SubmitReleaseOrderInput
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
	router.GET("/api/v1/release-orders/:id/details", func(c *gin.Context) {
		offset, offsetErr := strconv.Atoi(c.DefaultQuery("offset", "0"))
		limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "20"))
		if offsetErr != nil || limitErr != nil {
			writeReleaseError(c, application.ErrReleaseInvalid)
			return
		}
		page, err := orders.DetailPage(c.Request.Context(), c.Param("id"), c.Query("expected_version"), offset, limit)
		if writeReleaseError(c, err) {
			return
		}
		sanitizeReleaseItems(page.Items)
		c.JSON(200, page)
	})
	router.GET("/api/v1/release-orders/:id", func(c *gin.Context) {
		order, err := orders.Get(c.Request.Context(), c.Param("id"))
		if writeReleaseError(c, err) {
			return
		}
		c.JSON(200, releaseHeaderResponse(order, orders.AllowedActions(c.Request.Context(), order.Workflow())))
	})
}
func respondReleaseWrite(c *gin.Context, orders *application.ReleaseOrders, result application.ReleaseOrder, status int) {
	current, err := orders.Get(c.Request.Context(), result.ID)
	if err != nil {
		writeReleaseError(c, application.ErrReleaseUnknown)
		return
	}
	if result.Version == current.Version {
		result.Approvals = current.Approvals
	}
	if result.Approvals == nil {
		result.Approvals = []application.ReleaseTableApproval{}
	}
	result.ApprovalContext = current.ApprovalContext
	c.JSON(status, releaseResponse(result, orders.AllowedActions(c.Request.Context(), current.Workflow())))
}
func releaseResponse(order application.ReleaseOrder, actions []string) any {
	if order.RollbackTableFlows == nil {
		order.RollbackTableFlows = []application.ReleaseTableFlow{}
	}
	if order.Executions == nil {
		order.Executions = []application.ReleaseExecution{}
	}
	// Internal execution metadata and database identity are persisted, not client input.
	order.FrozenTables = nil
	sanitizeReleaseItems(order.Items)
	return struct {
		application.ReleaseOrder
		ItemCount       int            `json:"item_count"`
		OperationCounts map[string]int `json:"operation_counts"`
		AllowedActions  []string       `json:"allowed_actions"`
	}{order, len(order.Items), order.Summary().OperationCounts, actions}
}
func writeReleaseError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var failure *application.ReleaseExecutionFailure
	if errors.As(err, &failure) {
		c.Set("release_execution_failure", failure.HistorySaved)
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
	case errors.Is(err, application.ErrReleaseEmergencyReason):
		status, code, message = 422, "release_emergency_reason", err.Error()
	case errors.Is(err, application.ErrReleaseFlowIncomplete):
		status, code, message = 409, "release_flow_incomplete", "save valid flow instances for every table before submission"
	case errors.Is(err, application.ErrReleaseApproverUnavailable):
		status, code, message = 422, "release_approver_unavailable", err.Error()
	case errors.Is(err, application.ErrReleaseApprovalConflict):
		status, code, message = 409, "release_approval_conflict", "approval progress, qualification or confirmed scope changed; review the latest state"
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
		status, code, message = 422, "release_cross_table", "a derived detail must retain its original table identity"
	case errors.Is(err, application.ErrReleaseItemLimit):
		status, code, message = 422, "release_item_limit", "a release order must contain 1 to 1000 items"
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
		status, code, message = 503, "release_result_unknown", "commit outcome is unknown; read the order or manually repeat the original request"
	case errors.Is(err, application.ErrReleaseUnavailable):
	default:
		return writeManagedMutationError(c, err)
	}
	writeError(c, status, code, message)
	return true
}

func releaseHeaderResponse(header application.ReleaseHeader, actions []string) any {
	if header.RollbackTableFlows == nil {
		header.RollbackTableFlows = []application.ReleaseTableFlow{}
	}
	header.FrozenTables = nil
	return struct {
		application.ReleaseHeader
		AllowedActions []string `json:"allowed_actions"`
	}{header, actions}
}
func sanitizeReleaseItems(items []application.ReleaseItem) {
	for i := range items {
		items[i].ConcurrencyKeys = nil
		items[i].RecordKey = nil
		items[i].RecordTable = ""
	}
}
