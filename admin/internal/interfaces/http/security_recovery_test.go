package http

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise the public HTTP boundary with a panicking route. Publication-specific
// unavailable/timeout/unknown failures use actual MySQL in cmd/admin acceptance.
func TestRecoveryRedactsPanics(t *testing.T) {
	router := gin.New()
	router.Use(safeRecovery())
	router.POST("/panic", func(*gin.Context) { panic("driver detail secret-row-value") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/panic", nil))
	if response.Code != 500 || !strings.Contains(response.Body.String(), `"code":"internal_error"`) || strings.Contains(response.Body.String(), "secret") {
		t.Fatal(response.Code, response.Body)
	}
}
