package http

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	Authentication *application.Authentication
	AccountHTTP    AccountHTTPOptions
	APIToken       string
	AuthDisabled   bool
	CORSOrigins    []string
	AccessLog      io.Writer
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

func exactCORS(options RouterOptions) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(options.CORSOrigins))
	for _, origin := range options.CORSOrigins {
		allowedOrigins[origin] = struct{}{}
	}
	allowedMethods := map[string]struct{}{
		stdhttp.MethodGet: {}, stdhttp.MethodPost: {}, stdhttp.MethodPut: {},
		stdhttp.MethodPatch: {}, stdhttp.MethodDelete: {},
	}
	allowedHeaders := map[string]struct{}{
		"authorization": {}, "content-type": {}, "x-request-id": {},
	}
	return func(context *gin.Context) {
		if !isAPIRequest(context.Request.URL.Path) {
			context.Next()
			return
		}
		if isAccountRequest(context.Request.URL.Path) {
			context.Next()
			return
		}
		origin := context.GetHeader("Origin")
		if origin == "" {
			context.Next()
			return
		}
		if _, allowed := allowedOrigins[origin]; !allowed {
			writeError(context, stdhttp.StatusForbidden, "cors_origin_forbidden", "request origin is not allowed")
			context.Abort()
			return
		}
		context.Header("Access-Control-Allow-Origin", origin)
		context.Header("Vary", "Origin")

		requestedMethod := context.GetHeader("Access-Control-Request-Method")
		if context.Request.Method != stdhttp.MethodOptions || requestedMethod == "" {
			context.Next()
			return
		}
		if _, allowed := allowedMethods[requestedMethod]; !allowed || !validRequestedHeaders(context.GetHeader("Access-Control-Request-Headers"), allowedHeaders) {
			writeError(context, stdhttp.StatusForbidden, "cors_preflight_forbidden", "CORS preflight is not allowed")
			context.Abort()
			return
		}
		context.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
		context.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		context.Header("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
		context.Status(stdhttp.StatusNoContent)
		context.Abort()
	}
}

func validRequestedHeaders(header string, allowed map[string]struct{}) bool {
	if strings.TrimSpace(header) == "" {
		return true
	}
	for _, requested := range strings.Split(header, ",") {
		if _, found := allowed[strings.ToLower(strings.TrimSpace(requested))]; !found {
			return false
		}
	}
	return true
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

func bearerAuthentication(options RouterOptions) gin.HandlerFunc {
	expected := sha256.Sum256([]byte(options.APIToken))
	return func(context *gin.Context) {
		if !isAPIRequest(context.Request.URL.Path) || isAccountRequest(context.Request.URL.Path) || options.AuthDisabled {
			context.Next()
			return
		}
		authorization := context.GetHeader("Authorization")
		provided, found := strings.CutPrefix(authorization, "Bearer ")
		providedHash := sha256.Sum256([]byte(provided))
		if !found || provided == "" || subtle.ConstantTimeCompare(providedHash[:], expected[:]) != 1 {
			writeError(context, stdhttp.StatusUnauthorized, "unauthorized", "valid Bearer authentication is required")
			context.Abort()
			return
		}
		context.Next()
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
