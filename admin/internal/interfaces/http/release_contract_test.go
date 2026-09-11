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
	registerTableApprovalRoutes(router, nil)
	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{"GET /api/v1/table-policies/:table_name/approval-roles", "PUT /api/v1/table-policies/:table_name/approval-roles", "POST /api/v1/release-orders", "POST /api/v1/release-orders/preview", "PUT /api/v1/release-orders/:id", "GET /api/v1/release-orders", "GET /api/v1/release-orders/:id", "GET /api/v1/release-orders/:id/people", "GET /api/v1/release-orders/:id/details", "POST /api/v1/release-orders/:id/submit", "POST /api/v1/release-orders/:id/approve", "POST /api/v1/release-orders/:id/reject", "POST /api/v1/release-orders/:id/cancel", "POST /api/v1/release-orders/:id/copy", "POST /api/v1/release-orders/:id/execute", "POST /api/v1/release-orders/:id/complete", "POST /api/v1/release-orders/:id/quick-rollback", "POST /api/v1/release-orders/:id/quick-rollback/preview", "POST /api/v1/release-orders/:id/rollback-reason", "POST /api/v1/release-orders/:id/reprepare"} {
		if !routes[route] {
			t.Errorf("missing public release contract %s", route)
		}
	}
	if routes["POST /api/v1/release-orders/:id/rollback"] {
		t.Fatal("standalone rollback route must be deleted")
	}
	if routes["DELETE /api/v1/release-orders/:id"] {
		t.Fatal("release history must not have physical deletion")
	}
}
