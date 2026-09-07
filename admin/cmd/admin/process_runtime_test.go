package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

// The HTTP server owns the shutdown deadline. Give the external process time
// to finish that deadline and its final application cleanup before killing it.
const normalProcessShutdownWait = productionShutdownTimeout + 2*time.Second

func TestAdminProcessHelper(t *testing.T) {
	if os.Getenv("RCC_ADMIN_PROCESS_HELPER") != "true" {
		return
	}
	address := os.Getenv("RCC_ADMIN_PROCESS_ADDRESS")
	marker := os.Getenv("RCC_ADMIN_PROCESS_CLOSE_MARKER")
	shutdownTimeout, err := time.ParseDuration(os.Getenv("RCC_ADMIN_PROCESS_SHUTDOWN_TIMEOUT"))
	if err != nil {
		os.Exit(2)
	}

	handler := processTestRouter(t)
	if os.Getenv("RCC_ADMIN_PROCESS_MODE") == "blocking" {
		base := handler
		handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/block" {
				base.ServeHTTP(writer, request)
				return
			}
			writer.WriteHeader(http.StatusOK)
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
			<-request.Context().Done()
		})
	}

	app := &processTestApplication{handler: handler, closeMarker: marker}
	if err := serveAdmin(app, address, shutdownTimeout); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestAdminExternalProcessServesHealthAndEnforcesAuthDefault(t *testing.T) {
	process, address := startAdminProcessHelper(t, "normal", productionShutdownTimeout)

	assertProcessHTTP(t, address+"/health/live", "", http.StatusOK)
	assertProcessHTTP(t, address+"/health/ready", "", http.StatusOK)
	assertProcessHTTP(t, address+"/api/v1/database-tables", "", http.StatusUnauthorized)
	assertProcessHTTP(t, address+"/api/v1/database-tables", "Bearer process-token", http.StatusUnauthorized)

	stopProcessAndAssertClose(t, process, os.Interrupt, normalProcessShutdownWait)
}

func TestAdminExternalProcessHandlesSIGINTAndSIGTERMGracefully(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process signals are unavailable on Windows")
	}
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			process, address := startAdminProcessHelper(t, "normal", productionShutdownTimeout)
			// A connected client with an unfinished request header is real shutdown
			// work: net/http may retain StateNew until the read-header deadline.
			pending, err := net.Dial("tcp", address[len("http://"):])
			if err != nil {
				t.Fatalf("connect pending request: %v", err)
			}
			defer pending.Close()
			if _, err := io.WriteString(pending, "GET /health/live HTTP/1.1\r\n"); err != nil {
				t.Fatalf("write pending request header: %v", err)
			}
			assertProcessHTTP(t, address+"/health/live", "", http.StatusOK)
			// Allow the production shutdown window plus bounded process-exit overhead.
			stopProcessAndAssertClose(t, process, signal, normalProcessShutdownWait)
		})
	}
}

func TestAdminExternalProcessForcesShutdownAtConfiguredBoundAndStillClosesApplication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process signals are unavailable on Windows")
	}
	process, address := startAdminProcessHelper(t, "blocking", 100*time.Millisecond)
	assertProcessHTTP(t, address+"/block", "", http.StatusOK)
	started := time.Now()
	stopProcessAndAssertClose(t, process, syscall.SIGTERM, 2*time.Second)
	// External-process startup/teardown and race instrumentation add scheduling
	// overhead beyond the helper's 100 ms shutdown deadline. Keep the assertion
	// well below the production 10 second bound without making the race run flaky.
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("bounded shutdown took %v; output: %s", elapsed, process.output.String())
	}
}

func TestAdminExternalProcessWaitsForIncompleteRequestHeadersOnShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process signals are unavailable on Windows")
	}
	process, address := startAdminProcessHelper(t, "normal", productionShutdownTimeout)
	started := time.Now()
	connection, err := net.DialTimeout("tcp", strings.TrimPrefix(address, "http://"), time.Second)
	if err != nil {
		t.Fatalf("open incomplete request connection: %v", err)
	}
	defer connection.Close()
	if _, err := io.WriteString(connection, "GET /health/live HTTP/1.1\r\nHost: localhost\r\n"); err != nil {
		t.Fatalf("send incomplete request headers: %v", err)
	}
	// Keep the incomplete request open while checking that the server is live.
	// The elapsed-time assertion below also rejects a fast, vacuous pass where
	// this connection never participated in graceful shutdown.
	assertProcessHTTP(t, address+"/health/live", "", http.StatusOK)
	stopProcessAndAssertClose(t, process, os.Interrupt, normalProcessShutdownWait)
	if elapsed := time.Since(started); elapsed < readHeaderTimeout/2 {
		t.Fatalf("incomplete request did not delay shutdown: %v; output: %s", elapsed, process.output.String())
	}
}

func startAdminProcessHelper(t *testing.T, mode string, shutdownTimeout time.Duration) (*processHarness, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve process address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release process address: %v", err)
	}

	marker := filepath.Join(t.TempDir(), "application-closed")
	process := exec.Command(os.Args[0], "-test.run=^TestAdminProcessHelper$")
	process.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"RCC_ADMIN_PROCESS_HELPER=true",
		"RCC_ADMIN_PROCESS_ADDRESS=" + address,
		"RCC_ADMIN_PROCESS_CLOSE_MARKER=" + marker,
		"RCC_ADMIN_PROCESS_SHUTDOWN_TIMEOUT=" + shutdownTimeout.String(),
		"RCC_ADMIN_PROCESS_MODE=" + mode,
	}
	output := &synchronizedBuffer{}
	process.Stdout = output
	process.Stderr = output
	if err := process.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	harness := &processHarness{command: process, output: output, closeMarker: marker}
	t.Cleanup(func() {
		_ = harness.killAndWait()
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		request, _ := http.NewRequest(http.MethodGet, "http://"+address+"/health/live", nil)
		client := http.Client{Timeout: 100 * time.Millisecond}
		if response, err := client.Do(request); err == nil {
			_ = response.Body.Close()
			return harness, "http://" + address
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("helper process did not become live; output: %s", output.String())
	return nil, ""
}

func stopProcessAndAssertClose(t *testing.T, process *processHarness, signal os.Signal, timeout time.Duration) {
	t.Helper()
	if err := process.command.Process.Signal(signal); err != nil {
		t.Fatalf("signal helper process: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- process.wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper process exited unsuccessfully: %v; output: %s", err, process.output.String())
		}
	case <-time.After(timeout):
		_ = process.command.Process.Kill()
		waitErr := <-done
		t.Fatalf("helper process exceeded shutdown bound (wait error: %v); output: %s", waitErr, process.output.String())
	}
	contents, err := os.ReadFile(process.closeMarker)
	if err != nil || string(contents) != "closed" {
		t.Fatalf("application close evidence missing: contents=%q err=%v; output: %s", contents, err, process.output.String())
	}
}

type synchronizedBuffer struct {
	lock   sync.Mutex
	buffer bytes.Buffer
}

func (buffer *synchronizedBuffer) Write(data []byte) (int, error) {
	buffer.lock.Lock()
	defer buffer.lock.Unlock()
	return buffer.buffer.Write(data)
}

func (buffer *synchronizedBuffer) String() string {
	buffer.lock.Lock()
	defer buffer.lock.Unlock()
	return buffer.buffer.String()
}

type processHarness struct {
	command     *exec.Cmd
	output      *synchronizedBuffer
	closeMarker string
	waitOnce    sync.Once
	waitErr     error
}

func (process *processHarness) wait() error {
	process.waitOnce.Do(func() {
		process.waitErr = process.command.Wait()
	})
	return process.waitErr
}

func (process *processHarness) killAndWait() error {
	if process.command.Process != nil {
		_ = process.command.Process.Kill()
	}
	return process.wait()
}

func assertProcessHTTP(t *testing.T, url, authorization string, status int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create process request: %v", err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatalf("call process endpoint: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("expected HTTP %d, got %d: %s", status, response.StatusCode, body)
	}
}

type processTestApplication struct {
	handler     http.Handler
	closeMarker string
	once        sync.Once
}

func (application *processTestApplication) Handler() http.Handler { return application.handler }

func (application *processTestApplication) Close() error {
	var closeErr error
	application.once.Do(func() {
		closeErr = os.WriteFile(application.closeMarker, []byte("closed"), 0o600)
	})
	return closeErr
}

func processTestRouter(t *testing.T) http.Handler {
	t.Helper()
	metadata := processMetadata{}
	return httpinterface.NewRouter(application.NewDatabaseTableDiscovery(metadata), processReadiness{}, nil, nil, nil, nil, nil, httpinterface.RouterOptions{
		AccessLog: io.Discard,
	})
}

type processMetadata struct{}

func (processMetadata) ListDatabaseTables(context.Context) ([]domain.DatabaseTable, error) {
	return nil, nil
}
func (processMetadata) GetDatabaseTable(context.Context, string) (domain.DatabaseTable, error) {
	return domain.DatabaseTable{}, application.ErrDatabaseTableNotFound
}
func (processMetadata) GetTableSchema(context.Context, string) (domain.TableSchema, error) {
	return domain.TableSchema{}, application.ErrDatabaseTableNotFound
}

type processReadiness struct{}

func (processReadiness) Ready(context.Context) error { return nil }

var _ runtimeApplication = (*processTestApplication)(nil)
var _ application.TableMetadataReader = processMetadata{}
var _ application.Readiness = processReadiness{}

func decodeProcessError(body io.Reader) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(body).Decode(&envelope)
	return envelope.Error.Code
}
