// Package httpapi exposes Admin's policy-driven table API over Gin.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"github.com/gin-gonic/gin"
)

const maxRequestBodyBytes = 1 << 20

type pinger interface {
	PingContext(context.Context) error
}

// Handler serves the managed-table application service.
type Handler struct {
	service *application.Service
	catalog *application.CatalogService
	db      pinger
}

// NewRouter builds the complete Admin HTTP router.
func NewRouter(service *application.Service, catalog *application.CatalogService, db pinger) *gin.Engine {
	handler := &Handler{service: service, catalog: catalog, db: db}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	_ = router.SetTrustedProxies(nil)

	router.GET("/healthz", handler.health)
	router.GET("/readyz", handler.ready)

	api := router.Group("/api/v1")
	api.GET("/tables", handler.listTables)
	api.GET("/tables/:resource", handler.describeTable)
	api.POST("/tables/:resource/query", handler.queryRows)
	api.POST("/tables/:resource/rows", handler.createRow)
	api.PATCH("/tables/:resource/rows/:key", handler.updateRow)
	api.DELETE("/tables/:resource/rows/:key", handler.deleteRow)

	api.GET("/policies", handler.listPolicies)
	api.GET("/policies/:resource", handler.describePolicy)
	api.PUT("/policies/:resource", handler.savePolicy)
	api.DELETE("/policies/:resource", handler.deletePolicy)
	return router
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) ready(c *gin.Context) {
	if h.db == nil || h.db.PingContext(c.Request.Context()) != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (h *Handler) listTables(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.service.List()})
}

func (h *Handler) describeTable(c *gin.Context) {
	definition, err := h.service.Describe(c.Param("resource"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": definition})
}

func (h *Handler) queryRows(c *gin.Context) {
	var request domain.QuerySpec
	if err := decodeJSON(c, &request); err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.Query(c.Request.Context(), c.Param("resource"), request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result.Rows, "page": result.Page})
}

type mutationRequest struct {
	Values map[string]any `json:"values"`
}

func (h *Handler) createRow(c *gin.Context) {
	var request mutationRequest
	if err := decodeJSON(c, &request); err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.Create(c.Request.Context(), c.Param("resource"), request.Values)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) updateRow(c *gin.Context) {
	var request mutationRequest
	if err := decodeJSON(c, &request); err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.Update(c.Request.Context(), c.Param("resource"), c.Param("key"), request.Values)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) deleteRow(c *gin.Context) {
	result, err := h.service.Delete(c.Request.Context(), c.Param("resource"), c.Param("key"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Policy Catalog endpoints manage the catalog itself through dedicated
// capabilities; the catalog is not reachable through the generic table API.

func (h *Handler) listPolicies(c *gin.Context) {
	policies, err := h.catalog.List(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": policies})
}

func (h *Handler) describePolicy(c *gin.Context) {
	policy, err := h.catalog.Get(c.Request.Context(), c.Param("resource"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": policy})
}

func (h *Handler) savePolicy(c *gin.Context) {
	var policy domain.Policy
	if err := decodeJSON(c, &policy); err != nil {
		h.writeError(c, err)
		return
	}
	saved, err := h.catalog.Save(c.Request.Context(), c.Param("resource"), policy)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": saved})
}

func (h *Handler) deletePolicy(c *gin.Context) {
	if err := h.catalog.Delete(c.Request.Context(), c.Param("resource")); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"resource": c.Param("resource"), "deleted": true}})
}

func decodeJSON(c *gin.Context, destination any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return domain.Invalid("body", "invalid JSON: "+err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.Invalid("body", "must contain exactly one JSON object")
	}
	return nil
}

func (h *Handler) writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "internal server error"
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		status = http.StatusBadRequest
		code = "INVALID_ARGUMENT"
		message = validation.Error()
	case errors.Is(err, domain.ErrUnknownResource):
		status = http.StatusNotFound
		code = "TABLE_NOT_FOUND"
		message = err.Error()
	case errors.Is(err, domain.ErrPolicyNotFound):
		status = http.StatusNotFound
		code = "POLICY_NOT_FOUND"
		message = err.Error()
	case errors.Is(err, domain.ErrRowNotFound):
		status = http.StatusNotFound
		code = "ROW_NOT_FOUND"
		message = err.Error()
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
		code = "CONFLICT"
		message = err.Error()
	case errors.Is(err, domain.ErrOperationNotAllowed):
		status = http.StatusMethodNotAllowed
		code = "OPERATION_NOT_ALLOWED"
		message = err.Error()
	default:
		slog.Error("admin request failed", "error", err)
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
