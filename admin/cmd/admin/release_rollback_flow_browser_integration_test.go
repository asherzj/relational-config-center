//go:build integration && browser

package main

import "testing"

func TestRollbackFlowBrowserSystemPath(t *testing.T) {
	runReleaseWorkflowBrowserSystemPath(t, "release-rollback-flows.cjs")
}
