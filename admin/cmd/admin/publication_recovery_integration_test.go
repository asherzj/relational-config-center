//go:build integration

package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// This wire proxy changes only the external MySQL connection. It forwards real
// statements and their real results; the commit fault consumes MySQL's success
// packet and closes the socket before the Admin driver receives that packet.
type publicationWireProxy struct {
	listener    net.Listener
	upstream    string
	mode        atomic.Int32 // 1 = metadata disconnect, 2 = metadata stall, 3 = committed ACK loss
	ack         atomic.Uint32
	connections sync.Map
}

func newPublicationWireProxy(t *testing.T, upstream string) *publicationWireProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &publicationWireProxy{listener: listener, upstream: upstream}
	t.Cleanup(func() {
		listener.Close()
		p.connections.Range(func(k, v any) bool { k.(net.Conn).Close(); return true })
	})
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			go p.connect(client)
		}
	}()
	return p
}
func mysqlWirePacket(conn net.Conn) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	packet := make([]byte, length+4)
	copy(packet, header)
	_, err := io.ReadFull(conn, packet[4:])
	return packet, err
}
func (p *publicationWireProxy) connect(client net.Conn) {
	server, err := net.Dial("tcp", p.upstream)
	if err != nil {
		client.Close()
		return
	}
	p.connections.Store(client, true)
	p.connections.Store(server, true)
	defer func() { client.Close(); server.Close(); p.connections.Delete(client); p.connections.Delete(server) }()
	var published atomic.Bool
	var dropACK atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer client.Close()
		defer server.Close()
		for {
			packet, err := mysqlWirePacket(server)
			if err != nil {
				return
			}
			if dropACK.Swap(false) {
				if len(packet) > 4 && packet[4] == 0 {
					p.ack.Add(1)
				}
				return
			}
			if _, err = client.Write(packet); err != nil {
				return
			}
		}
	}()
	for {
		packet, err := mysqlWirePacket(client)
		if err != nil {
			return
		}
		if len(packet) > 5 && packet[3] == 0 && (packet[4] == 3 || packet[4] == 22) {
			statement := strings.ToUpper(string(packet[5:]))
			if strings.Contains(statement, "INSERT INTO RCC_REFRESH_NOTIFICATIONS") {
				published.Store(true)
			}
			if strings.Contains(statement, "INFORMATION_SCHEMA.INNODB_FOREIGN") {
				if p.mode.CompareAndSwap(1, 0) {
					return
				}
				if p.mode.CompareAndSwap(2, 0) {
					// A timed-out driver closes its socket. Observe that FIN so the
					// upstream transaction rolls back before an explicit retry.
					client.SetReadDeadline(time.Now().Add(3 * time.Second))
					var terminal [1]byte
					client.Read(terminal[:])
					return
				}
			}
			if strings.TrimSpace(statement) == "COMMIT" && published.Load() && p.mode.CompareAndSwap(3, 0) {
				dropACK.Store(true)
			}
		}
		if _, err = server.Write(packet); err != nil {
			return
		}
	}
}
func publicationProcessRequest(t *testing.T, p *accountProcess, path, key string, cookies []*http.Cookie, csrf string) (int, []byte) {
	t.Helper()
	status, _, body := p.requestWithKey(t, "POST", path, `{"expected_version":"3"}`, cookies, csrf, key)
	return status, body
}

// AC-031/036: distinguish dependency loss, request timeout, and a proven committed
// transaction whose ACK is lost. A genuinely new executable recovers the same
// key, cookies and original expected version from persistent storage.
func TestPublicationCommitUnknownSurvivesExecutableRestart(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"commit-loss","label":"committed once"}}]}`, "wire-publication")
	proxy := newPublicationWireProxy(t, driver.Addr)
	through := *driver
	through.Addr = proxy.listener.Addr().String()
	binary := buildIntegrationAdmin(t)
	process := accountProcessCommand(t, binary, &through, "MYSQL_READ_TIMEOUT=1s")
	process.ready(t)
	cookies, csrf, _ := processCredentials(t, process, "/api/v1/auth/login", `{"username":"integration.user","password":"correct horse battery staple"}`)
	for _, fault := range []struct {
		mode   int32
		status int
		code   string
	}{{1, 503, "release_unavailable"}, {2, 504, "mutation_timeout"}, {3, 503, "release_result_unknown"}} {
		proxy.mode.Store(fault.mode)
		status, body := publicationProcessRequest(t, process, path+"/execute", "original-execute", cookies, csrf)
		if status != fault.status || !strings.Contains(string(body), `"code":"`+fault.code+`"`) {
			t.Fatalf("fault %d: %d %s", fault.mode, status, body)
		}
		if proxy.mode.Load() != 0 {
			t.Fatal("wire fault did not reach target SQL")
		}
	}
	if proxy.ack.Load() != 1 {
		t.Fatal("test must consume exactly one actual COMMIT OK")
	}
	process.stop(t)
	process = accountProcessCommand(t, binary, &through, "MYSQL_READ_TIMEOUT=1s")
	process.ready(t)
	status, body := publicationProcessRequest(t, process, path+"/execute", "original-execute", cookies, csrf)
	var order domain.ReleaseOrder
	if status != 200 || json.Unmarshal(body, &order) != nil || order.State != "SUCCEEDED" || order.Publication == nil || len(order.Publication.Commands) != 1 {
		t.Fatalf("restart recovery: %d %s", status, body)
	}
	again, replay := publicationProcessRequest(t, process, path+"/execute", "original-execute", cookies, csrf)
	if again != 200 || string(replay) != string(body) {
		t.Fatal("replay changed durable result")
	}
	db := deliveryDB(t, driver)
	for _, query := range []string{`SELECT COUNT(*) FROM mutation_add_items WHERE code='commit-loss'`, `SELECT COUNT(*) FROM rcc_publication_commands`, `SELECT COUNT(*) FROM rcc_refresh_notifications`, `SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'execute:%'`, `SELECT COUNT(*) FROM rcc_table_publications WHERE table_version=1 AND command_cursor=1`} {
		var n int
		if err := db.QueryRow(query).Scan(&n); err != nil || n != 1 {
			t.Fatalf("durable count %d %v: %s", n, err, query)
		}
	}
	events := 0
	for _, event := range order.History {
		if event.Action == "EXECUTE" {
			events++
		}
	}
	if events != 1 || order.Publication.Notification.Status != "NOT_CONNECTED" {
		t.Fatal("false delivery or duplicate execution")
	}
	process.stop(t)
}
