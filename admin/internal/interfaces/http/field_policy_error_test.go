package http

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

// The HTTP adapter owns this stable status/code translation, independently of
// whether the deadline was encountered during metadata, persistence or readback.
func TestFieldPolicyErrorPreservesTimeoutContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"deadline", fmt.Errorf("driver operation: %w", context.DeadlineExceeded), 504, "field_policy_timeout"},
		{"dependency", errors.New("driver details must not escape"), 503, "field_policy_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(requestIDKey, "field-contract-request")
			if !fieldPolicyError(c, tc.err) {
				t.Fatal("error was ignored")
			}
			if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), tc.code) || !strings.Contains(recorder.Body.String(), "field-contract-request") {
				t.Fatalf("unstable timeout mapping: %d %s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "driver") {
				t.Fatal("driver details exposed")
			}
		})
	}
}
