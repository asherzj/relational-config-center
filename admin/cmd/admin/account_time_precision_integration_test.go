//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// Exercise the public credentials with clocks that MySQL would round upward,
// including a value that would carry into the next second.
func TestLocalAccountPersistedSessionDeadlines(t *testing.T) {
	var now time.Time
	app, db := accountFixture(t, func() time.Time { return now })
	type deadlines struct {
		Idle     time.Time `json:"idle_expires_at"`
		Absolute time.Time `json:"expires_at"`
	}
	readDeadlines := func(t *testing.T, response *httptest.ResponseRecorder) deadlines {
		t.Helper()
		var value deadlines
		if response.Code != 200 && response.Code != 201 {
			t.Fatalf("read session deadlines: status %d", response.Code)
		}
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if value.Idle.IsZero() || value.Absolute.IsZero() {
			t.Fatal("missing session deadlines")
		}
		return value
	}
	assertRestored := func(t *testing.T, response *httptest.ResponseRecorder) deadlines {
		t.Helper()
		issued := readDeadlines(t, response)
		restored := readDeadlines(t, accountRequest(app, "GET", "/api/v1/auth/session", "", response.Result().Cookies(), ""))
		if !issued.Idle.Equal(restored.Idle) || !issued.Absolute.Equal(restored.Absolute) {
			t.Fatalf("deadlines changed after persistence: issued=%+v restored=%+v", issued, restored)
		}
		return issued
	}
	for index, nanos := range []int{123456789, 999999999} {
		t.Run(fmt.Sprintf("nanoseconds_%d", nanos), func(t *testing.T) {
			now = time.Date(2026, 9, 7+index, 8, 0, 0, nanos, time.FixedZone("UTC+8", 8*60*60))
			username := fmt.Sprintf("precision.%d", index)
			password := "correct horse battery staple"
			registered := registerAccount(t, app, username, username+"@example.com", password)
			assertRestored(t, registered)
			loggedIn := loginAccount(t, app, username, password)
			assertRestored(t, loggedIn)

			now = now.Add(29 * time.Minute)
			activity := accountRequest(app, "POST", "/api/v1/auth/activity", "", registered.Result().Cookies(), sessionCSRF(t, registered))
			updated := readDeadlines(t, activity)
			persisted := readDeadlines(t, accountRequest(app, "GET", "/api/v1/auth/session", "", registered.Result().Cookies(), ""))
			if !updated.Idle.Equal(persisted.Idle) || !updated.Absolute.Equal(persisted.Absolute) {
				t.Fatalf("activity deadlines changed after persistence: updated=%+v persisted=%+v", updated, persisted)
			}
			now = updated.Idle.Add(-time.Nanosecond)
			if response := accountRequest(app, "GET", "/api/v1/auth/session", "", registered.Result().Cookies(), ""); response.Code != 200 {
				t.Fatalf("session rejected before advertised idle deadline: %d", response.Code)
			}
			now = updated.Idle
			for _, method := range []string{"GET", "POST"} {
				path := "/api/v1/auth/session"
				if method == "POST" {
					path = "/api/v1/auth/activity"
				}
				if response := accountRequest(app, method, path, "", registered.Result().Cookies(), sessionCSRF(t, registered)); response.Code != 401 {
					t.Fatalf("%s accepted at idle deadline: %d", method, response.Code)
				}
			}

			now = now.Add(789 * time.Nanosecond)
			long := loginAccount(t, app, username, password)
			initial := assertRestored(t, long)
			created := now
			for step := 1; step <= 16; step++ {
				now = created.Add(time.Duration(step) * 29 * time.Minute)
				response := accountRequest(app, "POST", "/api/v1/auth/activity", "", long.Result().Cookies(), sessionCSRF(t, long))
				if value := readDeadlines(t, response); !value.Absolute.Equal(initial.Absolute) {
					t.Fatalf("activity moved absolute deadline: %s -> %s", initial.Absolute, value.Absolute)
				}
			}
			now = initial.Absolute.Add(-time.Nanosecond)
			if response := accountRequest(app, "GET", "/api/v1/auth/session", "", long.Result().Cookies(), ""); response.Code != 200 {
				t.Fatalf("session rejected before advertised absolute deadline: %d", response.Code)
			}
			now = initial.Absolute
			if response := accountRequest(app, "GET", "/api/v1/auth/session", "", long.Result().Cookies(), ""); response.Code != 401 {
				t.Fatalf("session accepted at absolute deadline: %d", response.Code)
			}
			if response := accountRequest(app, "POST", "/api/v1/auth/activity", "", long.Result().Cookies(), sessionCSRF(t, long)); response.Code != 401 {
				t.Fatalf("activity accepted at absolute deadline: %d", response.Code)
			}
			// A successful authentication transaction must clean up at the same
			// deadline at which read-only current-session checks reject access.
			prepareAccount(t, app)
			var remaining int
			if err := db.QueryRow("SELECT COUNT(*) FROM rcc_login_sessions s JOIN rcc_accounts a ON a.id = s.account_id WHERE a.username = ?", username).Scan(&remaining); err != nil {
				t.Fatal(err)
			}
			if remaining != 0 {
				t.Fatalf("expired sessions survived cleanup: %d", remaining)
			}
		})
	}
}
