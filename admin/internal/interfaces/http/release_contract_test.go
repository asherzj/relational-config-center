package http

import (
	"github.com/gin-gonic/gin"
	"testing"
)

// Keep every implemented public action reachable through the same authenticated
// API composition, including the separate publication execution contract.
func TestReleaseWorkflowPublicRoutes(t *testing.T) {
	router := gin.New()
	registerReleaseOrderRoutes(router, nil)
	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{"POST /api/v1/release-orders", "POST /api/v1/release-orders/preview", "PUT /api/v1/release-orders/:id", "GET /api/v1/release-orders", "GET /api/v1/release-orders/:id", "GET /api/v1/release-orders/:id/people", "POST /api/v1/release-orders/:id/submit", "POST /api/v1/release-orders/:id/approve", "POST /api/v1/release-orders/:id/reject", "POST /api/v1/release-orders/:id/cancel", "POST /api/v1/release-orders/:id/copy", "POST /api/v1/release-orders/:id/execute", "POST /api/v1/release-orders/:id/complete", "POST /api/v1/release-orders/:id/rollback"} {
		if !routes[route] {
			t.Errorf("missing public release contract %s", route)
		}
	}
	if routes["DELETE /api/v1/release-orders/:id"] {
		t.Fatal("release history must not have physical deletion")
	}
}
