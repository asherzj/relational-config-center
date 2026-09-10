package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The public route registry determines which envelopes accept whole-order
// details. Streaming and declared-length requests must have the same boundary.
func TestReleaseDetailCapacityRouteContract(t *testing.T) {
	router := gin.New()
	router.Use(limitRequestBody(), func(c *gin.Context) {
		if _, err := io.Copy(io.Discard, c.Request.Body); err != nil {
			writeError(c, http.StatusBadRequest, "request_body_too_large", "body limit")
		} else {
			c.Status(http.StatusNoContent)
		}
		c.Abort()
	})
	registerReleaseOrderRoutes(router, nil)
	router.POST("/api/v1/tables/:name/query", func(c *gin.Context) {})
	for _, test := range []struct {
		method, path string
		accepted     bool
	}{
		{"POST", "/api/v1/release-orders", true},
		{"POST", "/api/v1/release-orders/preview", true},
		{"PUT", "/api/v1/release-orders/order", true},
		{"POST", "/api/v1/release-orders/order/copy", true},
		{"POST", "/api/v1/release-orders/order/reprepare", true},
		{"POST", "/api/v1/release-orders/order/execute", false},
		{"POST", "/api/v1/release-orders/order/quick-rollback", false},
		{"POST", "/api/v1/release-orders/unknown/action", false},
		{"PUT", "/api/v1/release-orders/order/unknown", false},
		{"POST", "/api/v1/tables/table/query", false},
	} {
		for _, chunked := range []bool{false, true} {
			t.Run(test.method+" "+test.path+map[bool]string{false: " declared", true: " streamed"}[chunked], func(t *testing.T) {
				request := httptest.NewRequest(test.method, test.path, strings.NewReader(strings.Repeat("x", (1<<20)+1)))
				if chunked {
					request.ContentLength = -1
					request.TransferEncoding = []string{"chunked"}
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				want := http.StatusBadRequest
				if test.accepted {
					want = http.StatusNoContent
				}
				if response.Code != want {
					t.Fatalf("status %d want %d: %s", response.Code, want, response.Body.String())
				}
			})
		}
	}
}
