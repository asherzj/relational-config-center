//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestLocalAccountSecurityChangesWinAgainstVerifiedLogin(t *testing.T) {
	f := newMaintenanceFixture(t)
	oldPassword := "current password long enough"
	hash := encodedTestPassword(oldPassword, 65536, 5)
	for _, change := range []string{"no-security-change", "logout-all", "password-change", "same-password-change", "reset-password", "disable", "disable-enable"} {
		t.Run(change, func(t *testing.T) {
			username := "race." + change
			current := registerAccount(t, f.app, username, username+"@example.com", oldPassword)
			if _, err := f.db.Exec("UPDATE rcc_accounts SET password_hash = ? WHERE username = ?", hash, username); err != nil {
				t.Fatal(err)
			}
			cookies, csrf := prepareAccount(t, f.app)
			verified, release := make(chan struct{}), make(chan struct{})
			var signal, unblock sync.Once
			resume := func() { unblock.Do(func() { close(release) }) }
			t.Cleanup(resume)
			clockErrors := make(chan error, 1)
			// This second Admin uses the real repository and Argon2 verifier. Login
			// calls the clock before reserving in_flight, then next in newSession
			// after verifying the password and enabled snapshot. Waiting in that
			// next clock call pins the verified old state before IssueSession.
			// Merely observing the reservation would not establish that ordering.
			// The identical no-security-change case must issue a working session,
			// so a failed verifier cannot masquerade as a successful race test.
			clock := func() time.Time {
				var inFlight int
				err := f.db.QueryRow("SELECT COALESCE(SUM(in_flight),0) FROM rcc_auth_rate_limits WHERE bucket_key LIKE 'login-user:%'").Scan(&inFlight)
				if err != nil {
					select {
					case clockErrors <- err:
					default:
					}
				}
				if inFlight == 1 {
					signal.Do(func() { close(verified); <-release })
				}
				return time.Now()
			}
			loginApp, err := newApplicationWithClock(context.Background(), f.settings, clock)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loginApp.Close() })
			body, _ := json.Marshal(map[string]string{"username": username, "password": oldPassword})
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- accountRequest(loginApp, "POST", "/api/v1/auth/login", string(body), cookies, csrf) }()
			select {
			case <-verified:
			case err := <-clockErrors:
				t.Fatal(err)
			case response := <-result:
				t.Fatalf("login did not reach verified-state barrier: %d", response.Code)
			case <-time.After(3 * time.Second):
				t.Fatal("login did not reach verified-state barrier")
			}
			newPassword := oldPassword
			switch change {
			case "logout-all":
				response := accountRequest(f.app, "POST", "/api/v1/auth/logout-all", "", current.Result().Cookies(), sessionCSRF(t, current))
				if response.Code != 204 {
					t.Fatalf("logout-all: %d %s", response.Code, response.Body.String())
				}
			case "password-change", "same-password-change":
				if change == "password-change" {
					newPassword = "replacement password long enough"
				}
				body, _ := json.Marshal(map[string]string{"current_password": oldPassword, "new_password": newPassword})
				response := accountRequest(f.app, "POST", "/api/v1/auth/password", string(body), current.Result().Cookies(), sessionCSRF(t, current))
				if response.Code != 204 {
					t.Fatalf("password-change: %d %s", response.Code, response.Body.String())
				}
			case "reset-password":
				newPassword = "replacement password long enough"
				f.run(t, newPassword, "reset-password", "--username", username, "--password-stdin")
			case "disable", "disable-enable":
				f.run(t, "", "disable", "--username", username)
				if change == "disable-enable" {
					f.run(t, "", "enable", "--username", username)
				}
			}
			resume()
			var response *httptest.ResponseRecorder
			select {
			case response = <-result:
			case <-time.After(3 * time.Second):
				t.Fatal("login did not resume")
			}
			if change == "no-security-change" {
				if response.Code != 200 {
					t.Fatalf("positive control failed: %d %s", response.Code, response.Body.String())
				}
				if current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", response.Result().Cookies(), ""); current.Code != 200 {
					t.Fatalf("positive control issued unusable session: %d", current.Code)
				}
			} else if response.Code != 401 || len(response.Result().Cookies()) != 0 {
				t.Fatalf("old state issued session: %d %s", response.Code, response.Body.String())
			}
			if change == "disable" {
				cookies, csrf := prepareAccount(t, f.app)
				if response := accountRequest(f.app, "POST", "/api/v1/auth/login", string(body), cookies, csrf); response.Code != 401 {
					t.Fatalf("new login ignored disabled status: %d", response.Code)
				}
			} else {
				loginAccount(t, f.app, username, newPassword)
			}
		})
	}
}
