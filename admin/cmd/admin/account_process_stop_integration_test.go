//go:build integration

package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAccountProcessStopAllowsPendingRequestHeader(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process signals are unavailable on Windows")
	}
	for _, pendingHeader := range []bool{false, true} {
		name := "idle"
		if pendingHeader {
			name = "pending_header"
		}
		t.Run(name, func(t *testing.T) {
			harness, origin := startAdminProcessHelper(t, "normal", productionShutdownTimeout)
			process := &accountProcess{cmd: harness.command, output: harness.output, done: make(chan struct{}), origin: origin}
			go func() { process.waitErr = harness.wait(); close(process.done) }()
			if pendingHeader {
				// Exercise the account test's actual stop helper with real HTTP
				// shutdown work, without introducing MySQL/container startup.
				pending, err := net.Dial("tcp", strings.TrimPrefix(origin, "http://"))
				if err != nil {
					t.Fatalf("connect pending request: %v", err)
				}
				defer pending.Close()
				if _, err := io.WriteString(pending, "GET /health/live HTTP/1.1\r\n"); err != nil {
					t.Fatalf("write pending request header: %v", err)
				}
			}
			assertProcessHTTP(t, origin+"/health/live", "", http.StatusOK)
			started := time.Now()
			process.stop(t)
			if elapsed := time.Since(started); elapsed > productionShutdownTimeout+2*time.Second {
				t.Fatalf("account process exceeded shutdown bound: %v", elapsed)
			}
			contents, err := os.ReadFile(harness.closeMarker)
			if err != nil || string(contents) != "closed" {
				t.Fatalf("application close evidence missing: contents=%q err=%v", contents, err)
			}
		})
	}
}
