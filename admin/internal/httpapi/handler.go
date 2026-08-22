// Package httpapi exposes Admin's policy-driven table API over Gin.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/asherzj/relational-config-center/admin/internal/managedtable"
	"github.com/gin-gonic/gin"
)

const maxRequestBodyBytes = 1 << 20

type pinger interface {
	PingContext(context.Context) error
}

// Handler serves the managed-table application service.
type Handler struct {
	service *managedtable.Service
	db      pinger
}

// NewRouter builds the complete Admin HTTP router.
func NewRouter(service *managedtable.Service, db pinger) *gin.Engine {
	handler := &Handler{service: service, db: db}
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
	var request managedtable.QuerySpec
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

func decodeJSON(c *gin.Context, destination any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return managedtable.Invalid("body", "invalid JSON: "+err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return managedtable.Invalid("body", "must contain exactly one JSON object")
	}
	return nil
}

func (h *Handler) writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "internal server error"
	var validation *managedtable.ValidationError
	switch {
	case errors.As(err, &validation):
		status = http.StatusBadRequest
		code = "INVALID_ARGUMENT"
		message = validation.Error()
	case errors.Is(err, managedtable.ErrUnknownResource):
		status = http.StatusNotFound
		code = "TABLE_NOT_FOUND"
		message = err.Error()
	case errors.Is(err, managedtable.ErrRowNotFound):
		status = http.StatusNotFound
		code = "ROW_NOT_FOUND"
		message = err.Error()
	case errors.Is(err, managedtable.ErrConflict):
		status = http.StatusConflict
		code = "CONFLICT"
		message = err.Error()
	case errors.Is(err, managedtable.ErrOperationNotAllowed):
		status = http.StatusMethodNotAllowed
		code = "OPERATION_NOT_ALLOWED"
		message = err.Error()
	default:
		slog.Error("admin request failed", "error", err)
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
