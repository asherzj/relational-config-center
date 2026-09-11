//go:build integration && browser

package main

import "testing"

// Reuse the formal Admin process, same-origin Web proxy and disposable MySQL
// fixture. Business operations and result acknowledgements use public HTTP.
func TestReleaseNotificationsBrowserSystemPath(t *testing.T) {
	runNotificationBrowserSystemPath(t, "release-notifications.cjs")
}
