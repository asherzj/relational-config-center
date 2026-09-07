package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/gin-gonic/gin"
)

const (
	requestIDHeader = "X-Request-ID"
	requestIDKey    = "request_id"
	maximumBodySize = int64(1 << 20)
)

var (
	requestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	requestIDFallback atomic.Uint64
)

// RouterOptions is the deployment-owned interface of the HTTP safety module.
// Request-scoped safety behavior remains behind NewRouter.
type RouterOptions struct {
	Authentication     *application.Authentication
	AccountRoles       *application.AccountRoleManagement
	ReleaseOrders      *application.ReleaseOrders
	PublicationTimeout time.Duration
	AccountHTTP        AccountHTTPOptions
	AccessLog          io.Writer
}

func publicationDeadline(timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.FullPath(), "/api/v1/release-orders") || c.Request.Method == "GET" {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func limitRequestBody() gin.HandlerFunc {
	return func(context *gin.Context) {
		if !isAPIRequest(context.Request.URL.Path) {
			context.Next()
			return
		}
		if context.Request.ContentLength > maximumBodySize {
			writeError(context, stdhttp.StatusBadRequest, "request_body_too_large", "request body exceeds the 1 MiB limit")
			context.Abort()
			return
		}
		if context.Request.Body != nil {
			context.Request.Body = stdhttp.MaxBytesReader(context.Writer, context.Request.Body, maximumBodySize)
		}
		context.Next()
	}
}

func safeRecovery() gin.HandlerFunc {
	return func(context *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}
			context.Abort()
			if context.Writer.Written() {
				return
			}
			writeError(context, stdhttp.StatusInternalServerError, "internal_error", "internal server error")
		}()
		context.Next()
	}
}

type accessLogEvent struct {
	RequestID  string `json:"request_id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"duration_ms"`
}

func structuredAccessLog(destination io.Writer) gin.HandlerFunc {
	if destination == nil {
		destination = io.Discard
	}
	var lock sync.Mutex
	encoder := json.NewEncoder(destination)
	return func(context *gin.Context) {
		if !isAPIRequest(context.Request.URL.Path) {
			context.Next()
			return
		}
		started := time.Now()
		context.Next()
		route := context.FullPath()
		if route == "" {
			route = "unmatched"
		}
		event := accessLogEvent{
			RequestID:  requestID(context),
			Method:     context.Request.Method,
			Path:       route,
			Status:     context.Writer.Status(),
			DurationMS: time.Since(started).Milliseconds(),
		}
		lock.Lock()
		_ = encoder.Encode(event)
		lock.Unlock()
	}
}

func requestIdentity() gin.HandlerFunc {
	return func(context *gin.Context) {
		if !isAPIRequest(context.Request.URL.Path) {
			context.Next()
			return
		}
		requestID := ""
		incoming := context.Request.Header.Values(requestIDHeader)
		if len(incoming) == 1 && requestIDPattern.MatchString(incoming[0]) {
			requestID = incoming[0]
		}
		if requestID == "" {
			requestID = newRequestID()
		}
		context.Set(requestIDKey, requestID)
		context.Header(requestIDHeader, requestID)
		context.Next()
	}
}

func sessionAuthentication(options RouterOptions) gin.HandlerFunc {
	h := accountHandler{options: options.AccountHTTP}
	return func(c *gin.Context) {
		if !isAPIRequest(c.Request.URL.Path) || isAccountRequest(c.Request.URL.Path) {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		if options.Authentication == nil {
			writeAuthError(c, application.ErrSession)
			c.Abort()
			return
		}
		timeout := options.AccountHTTP.RequestTimeout
		if timeout <= 0 {
			timeout = 4 * time.Second
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		change := c.Request.Method != stdhttp.MethodGet && c.Request.Method != stdhttp.MethodHead
		operator, err := options.Authentication.AuthenticateRequest(ctx, h.cookie(c, false), c.GetHeader("X-CSRF-Token"), change)
		if err == nil && change && !h.sameOrigin(c) {
			err = application.ErrCSRF
		}

		cancel()
		if err != nil {
			writeAuthError(c, err)
			c.Abort()
			return
		}
		// Query uses POST for structured conditions, but requires only read access.
		required := application.RoleViewer
		if change && c.FullPath() != "/api/v1/tables/:table_name/query" {
			required = application.RoleAdmin
			if c.FullPath() == "/api/v1/release-orders/:id/copy" || c.FullPath() == "/api/v1/release-orders/:id/submit" || c.FullPath() == "/api/v1/release-orders/preview" || c.FullPath() == "/api/v1/release-orders" || c.FullPath() == "/api/v1/release-orders/:id" || c.FullPath() == "/api/v1/release-orders/:id/cancel" {
				required = application.RoleEditor
			}
			if c.FullPath() == "/api/v1/release-orders/:id/execute" {
				required = application.RolePublisher
			}
			if c.FullPath() == "/api/v1/release-orders/:id/approve" || c.FullPath() == "/api/v1/release-orders/:id/reject" {
				required = application.RoleApprover
			}
		}
		if !operator.Allows(required) {
			writeError(c, 403, "permission_denied", "the current account does not have the required role")
			c.Abort()
			return
		}
		c.Header("X-RCC-Account-ID", operator.AccountID())
		c.Request = c.Request.WithContext(operator.Bind(c.Request.Context()))
		c.Next()
	}
}

func isAPIRequest(path string) bool {
	return path == "/api/v1" || strings.HasPrefix(path, "/api/v1/")
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		sequence := requestIDFallback.Add(1)
		return "rcc-fallback-" + strings.ToLower(hex.EncodeToString([]byte{
			byte(sequence >> 56), byte(sequence >> 48), byte(sequence >> 40), byte(sequence >> 32),
			byte(sequence >> 24), byte(sequence >> 16), byte(sequence >> 8), byte(sequence),
		}))
	}
	return "rcc-" + hex.EncodeToString(buffer)
}

func requestID(context *gin.Context) string {
	value, exists := context.Get(requestIDKey)
	if !exists {
		return ""
	}
	requestID, _ := value.(string)
	return requestID
}
