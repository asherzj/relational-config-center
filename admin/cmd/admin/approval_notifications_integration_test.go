//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func assertApprovalCounts(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, unread, pending int) {
	t.Helper()
	response := releaseActorRequest(t, app, actor, "GET", "/api/v1/approval-notifications", "", "")
	if response.Code != 200 {
		t.Fatalf("notification counts: %d %s", response.Code, response.Body)
	}
	var counts struct {
		Unread  int `json:"unread_count"`
		Pending int `json:"pending_count"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Unread != unread || counts.Pending != pending {
		t.Fatalf("notification counts: got %+v want unread=%d pending=%d", counts, unread, pending)
	}
}

// AC-014: only eligible accounts receive one aggregate regardless of roles/tables.
func TestApprovalNotificationsSubmitAggregatesCurrentRecipients(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "notify.member", "notify.member@example.com", "correct horse battery staple")
	viewer := registerAccount(t, app, "notify.viewer", "notify.viewer@example.com", "correct horse battery staple")
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "通知一", member), tableApprovalRole(t, app, "通知二", member))
	path := createTableApprovalDraft(t, app, admin, "notify-submit")
	assertApprovalCounts(t, app, member, 0, 0)
	for range 2 {
		rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "notify-submit"), 200)
	}
	assertApprovalCounts(t, app, member, 1, 1)
	assertApprovalCounts(t, app, viewer, 0, 0)
	assertApprovalCounts(t, app, admin, 0, 0)
}

// AC-015: partial/final progress aggregates for the applicant; other pending
// tables survive, failed decisions and self cancellation cannot invent updates.
func TestApprovalNotificationsApprovalProgressAndTerminalResults(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	goods := registerAccount(t, app, "progress.goods", "progress.goods@example.com", "correct horse battery staple")
	prices := registerAccount(t, app, "progress.prices", "progress.prices@example.com", "correct horse battery staple")
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "商品审批", goods, prices))
	assignTableApproval(t, app, "mutation_supplied_id_items", tableApprovalRole(t, app, "价格审批", prices))
	create := func(code string) string {
		body := fmt.Sprintf(`{"title":"审批进展","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":%q,"label":"intent"}},{"table_name":"mutation_supplied_id_items","operation":"ADD","content":{"id":%q,"label":"intent"}}]}`, code, code)
		order := rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", "/api/v1/release-orders", body, "progress-create-"+code), 201)
		path := "/api/v1/release-orders/" + order.ID
		rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "progress-submit-"+code), 200)
		return path
	}
	path := create("progress-first")
	rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", confirmedApprovalBody(t, app, goods, path, "商品通过"), "progress-goods"), 200)
	assertApprovalCounts(t, app, admin, 1, 0)
	assertApprovalCounts(t, app, goods, 0, 0)
	assertApprovalCounts(t, app, prices, 1, 1)
	rollbackOrderResponse(t, releaseActorRequest(t, app, prices, "POST", path+"/approve", confirmedApprovalBody(t, app, prices, path, "价格通过"), "progress-prices"), 200)
	assertApprovalCounts(t, app, admin, 1, 0)
	assertApprovalCounts(t, app, prices, 0, 0)
	path = create("progress-reject")
	rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/reject", confirmedApprovalBody(t, app, goods, path, "商品拒绝"), "progress-reject"), 200)
	assertApprovalCounts(t, app, admin, 2, 0)
	assertApprovalCounts(t, app, prices, 0, 0)
	path = create("progress-cancel")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/cancel", `{"expected_version":"2","reason":"自己取消"}`, "progress-cancel"), 200)
	assertApprovalCounts(t, app, admin, 2, 0)
	assertApprovalCounts(t, app, goods, 0, 0)
}

// AC-013: management and database-authorized maintenance reconcile existing
// pending responsibilities, including fallback ADMIN, without changing orders.
func TestApprovalNotificationsQualificationChangesAcrossEveryEntry(t *testing.T) {
	f := newMaintenanceFixture(t)
	fixture, err := os.ReadFile("testdata/006-mutation-fixture.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if strings.TrimSpace(statement) != "" {
			deliveryExec(t, f.databaseOwner, statement)
		}
	}
	app := f.app
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	first := registerAccount(t, app, "qual.first", "qual.first@example.com", "correct horse battery staple")
	second := registerAccount(t, app, "qual.second", "qual.second@example.com", "correct horse battery staple")
	fallback := registerAccount(t, app, "qual.fallback", "qual.fallback@example.com", "correct horse battery staple")
	role := tableApprovalRole(t, app, "资格角色", first)
	assignTableApproval(t, app, "mutation_add_items", role)
	path := createTableApprovalDraft(t, app, admin, "qual-pending")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "qual-submit"), 200)
	role = updateTableRole(t, app, role, "新成员接手", true, second)
	assertApprovalCounts(t, app, first, 0, 0)
	assertApprovalCounts(t, app, second, 1, 1)
	role = updateTableRole(t, app, role, "角色停用", false, second)
	assertApprovalCounts(t, app, second, 0, 0)
	f.run(t, "", "grant-admin", "--id", accountID(t, fallback))
	assertApprovalCounts(t, app, fallback, 1, 1)
	grantReleaseRole(t, app, fallback, `["VIEWER"]`, "2", "qual-demote")
	assertApprovalCounts(t, app, fallback, 0, 0)
	grantReleaseRole(t, app, fallback, `["ADMIN"]`, "3", "qual-grant")
	assertApprovalCounts(t, app, fallback, 1, 1)
	role = updateTableRole(t, app, role, "恢复成员", true, second)
	assertApprovalCounts(t, app, second, 1, 1)
	assertApprovalCounts(t, app, fallback, 0, 0)
	f.run(t, "", "disable", "--id", accountID(t, second))
	assertApprovalCounts(t, app, fallback, 1, 1)
	f.run(t, "", "enable", "--id", accountID(t, second))
	second = loginAccount(t, app, "qual.second", "correct horse battery staple")
	assertApprovalCounts(t, app, second, 1, 1)
	assertApprovalCounts(t, app, fallback, 0, 0)
	// Changing the table assignment does not rewrite this order's responsibility.
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "后来绑定", first))
	assertApprovalCounts(t, app, first, 0, 0)
	assertApprovalCounts(t, app, second, 1, 1)
	order := readTableApprovalOrder(t, app, second, path)
	if order.Version != "2" || len(order.History) != 2 {
		t.Fatal("qualification refresh edited the order")
	}
}

func readApprovalProgress(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, path string) domain.ApprovalNotification {
	t.Helper()
	response := releaseActorRequest(t, app, actor, "GET", path, "", "")
	if response.Code != 200 {
		t.Fatalf("detail notification: %d %s", response.Code, response.Body)
	}
	var detail struct {
		Notification domain.ApprovalNotification `json:"notification"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Notification.Sequence == "" {
		t.Fatal("successful detail lacks its observed notification progress")
	}
	return detail.Notification
}
func acknowledgeApprovalProgress(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, path, sequence string) {
	t.Helper()
	response := releaseActorRequest(t, app, actor, "POST", path+"/notification-read", fmt.Sprintf(`{"sequence":%q}`, sequence), "")
	if response.Code != 200 {
		t.Fatalf("ack: %d %s", response.Code, response.Body)
	}
}

// AC-016: the seq accompanying a successful detail, not a later count/list read,
// is the acknowledgement bound. Reads never consume pending work.
func TestApprovalNotificationsObservedDetailBoundsLateAcknowledgement(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "read.member", "read.member@example.com", "correct horse battery staple")
	role := tableApprovalRole(t, app, "读取进度", member)
	assignTableApproval(t, app, "mutation_add_items", role)
	path := createTableApprovalDraft(t, app, admin, "read-progress")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "read-submit"), 200)
	observed := readApprovalProgress(t, app, member, path)
	if observed.Sequence != "1" || !observed.Unread || !observed.Pending {
		t.Fatalf("first progress: %+v", observed)
	}
	acknowledgeApprovalProgress(t, app, member, path, observed.Sequence)
	assertApprovalCounts(t, app, member, 0, 1)
	// Removing and re-adding qualification creates a new independent notification,
	// without changing the release's version. Old detail acknowledgement stays old.
	role = updateTableRole(t, app, role, "暂时退出", false, member)
	role = updateTableRole(t, app, role, "再次接手", true, member)
	for range 2 {
		acknowledgeApprovalProgress(t, app, member, path, observed.Sequence)
	}
	assertApprovalCounts(t, app, member, 1, 1)
	current := readApprovalProgress(t, app, member, path)
	if current.Sequence != "2" {
		t.Fatalf("qualification sequence: %+v", current)
	}
	acknowledgeApprovalProgress(t, app, member, path, current.Sequence)
	assertApprovalCounts(t, app, member, 0, 1)
	// Read the applicant's header, then approve, then read the newer counts.
	// The earlier header must not acknowledge the unseen decision.
	applicantObserved := readApprovalProgress(t, app, admin, path)
	rollbackOrderResponse(t, releaseActorRequest(t, app, member, "POST", path+"/approve", confirmedApprovalBody(t, app, member, path, "后来批准"), "read-approve"), 200)
	assertApprovalCounts(t, app, admin, 1, 0)
	acknowledgeApprovalProgress(t, app, admin, path, applicantObserved.Sequence)
	assertApprovalCounts(t, app, admin, 1, 0)
	applicantCurrent := readApprovalProgress(t, app, admin, path)
	acknowledgeApprovalProgress(t, app, admin, path, applicantCurrent.Sequence)
	assertApprovalCounts(t, app, admin, 0, 0)
	assertNotificationOrders(t, readNotificationPage(t, app, admin, "view=mine&unread=true"))
	assertNotificationOrders(t, readNotificationPage(t, app, member, "view=handled"), path)
}

// AC-018 / AC-016: a failed durable notification rolls back the real operation;
// failure to acknowledge is independent and never prevents reading the detail.
func TestApprovalNotificationsFailuresRollbackBusinessAndReadProgress(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ownerConfig := *driver
	ownerConfig.User = "root"
	owner := deliveryDB(t, &ownerConfig)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "atomic.notice", "atomic.notice@example.com", "correct horse battery staple")
	role := tableApprovalRole(t, app, "事务提醒", member)
	assignTableApproval(t, app, "mutation_add_items", role)
	for _, action := range []string{"submit", "approve", "reject", "cancel"} {
		t.Run(action, func(t *testing.T) {
			path := createTableApprovalDraft(t, app, admin, "atomic-notice-"+action)
			actor := admin
			body := `{"expected_version":"1"}`
			if action != "submit" {
				rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", body, "atomic-pre-submit-"+action), 200)
				if action == "cancel" {
					body = `{"expected_version":"2","reason":"取消事务"}`
				} else {
					actor = member
					body = confirmedApprovalBody(t, app, member, path, "决定事务")
				}
			}
			before := readTableApprovalOrder(t, app, admin, path)
			observed := readApprovalProgress(t, app, member, path)
			deliveryExec(t, owner, `CREATE TRIGGER fail_notification_insert BEFORE INSERT ON rcc_approval_notifications FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected notification insert failure'`)
			deliveryExec(t, owner, `CREATE TRIGGER fail_notification_update BEFORE UPDATE ON rcc_approval_notifications FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected notification update failure'`)
			failed := releaseActorRequest(t, app, actor, "POST", path+"/"+action, body, "atomic-notification-"+action)
			assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
			deliveryExec(t, owner, `DROP TRIGGER fail_notification_insert`)
			deliveryExec(t, owner, `DROP TRIGGER fail_notification_update`)
			after := readTableApprovalOrder(t, app, admin, path)
			if after.Version != before.Version || after.State != before.State || len(after.History) != len(before.History) {
				t.Fatal("notification failure partially saved business")
			}
			if next := readApprovalProgress(t, app, member, path); next != observed {
				t.Fatalf("failed business changed notification: %+v -> %+v", observed, next)
			}
			// Repeated GETs do not recreate the failed operation or manufacture facts.
			assertNotificationOrders(t, readNotificationPage(t, app, admin, "view=all&id="+after.ID), func() []string {
				if action == "submit" {
					return nil
				}
				return []string{path}
			}()...)
			for range 2 {
				rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/"+action, body, "atomic-notification-"+action), 200)
			}
			saved := readTableApprovalOrder(t, app, admin, path)
			if len(saved.History) != len(before.History)+1 {
				t.Fatal("same request repeated the business event")
			}
			assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/"+action, strings.ReplaceAll(strings.ReplaceAll(body, `"expected_version":"1"`, `"expected_version":"999"`), `"expected_version":"2"`, `"expected_version":"999"`), "atomic-notification-"+action), 409, "idempotency_conflict")
		})
	}
	path := createTableApprovalDraft(t, app, admin, "atomic-read")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "atomic-read-submit"), 200)
	observed := readApprovalProgress(t, app, member, path)
	deliveryExec(t, owner, `CREATE TRIGGER fail_notification_ack BEFORE UPDATE ON rcc_approval_notifications FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected acknowledgement failure'`)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, member, "POST", path+"/notification-read", fmt.Sprintf(`{"sequence":%q}`, observed.Sequence), ""), 503, "release_unavailable")
	if next := readApprovalProgress(t, app, member, path); next != observed {
		t.Fatal("failed ack consumed unread or blocked detail")
	}
	deliveryExec(t, owner, `DROP TRIGGER fail_notification_ack`)
	acknowledgeApprovalProgress(t, app, member, path, observed.Sequence)
	if next := readApprovalProgress(t, app, member, path); next.Unread || !next.Pending {
		t.Fatal("successful retry confused unread with pending")
	}
}

func TestApprovalNotificationsQualificationFailureRollsBackEveryEntry(t *testing.T) {
	f := newMaintenanceFixture(t)
	fixture, err := os.ReadFile("testdata/006-mutation-fixture.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if strings.TrimSpace(statement) != "" {
			deliveryExec(t, f.databaseOwner, statement)
		}
	}
	app := f.app
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "failure.member", "failure.member@example.com", "correct horse battery staple")
	fallback := registerAccount(t, app, "failure.admin", "failure.admin@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, fallback, `["ADMIN"]`, "1", "failure-admin")
	role := tableApprovalRole(t, app, "资格失败", member)
	assignTableApproval(t, app, "mutation_add_items", role)
	path := createTableApprovalDraft(t, app, admin, "failure-qualified")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "failure-qualification-submit"), 200)
	fail := func() {
		deliveryExec(t, f.databaseOwner, `CREATE TRIGGER fail_qualification_insert BEFORE INSERT ON rcc_approval_notifications FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected qualification notification insert failure'`)
		deliveryExec(t, f.databaseOwner, `CREATE TRIGGER fail_qualification_update BEFORE UPDATE ON rcc_approval_notifications FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected qualification notification update failure'`)
	}
	restore := func() {
		deliveryExec(t, f.databaseOwner, `DROP TRIGGER fail_qualification_insert`)
		deliveryExec(t, f.databaseOwner, `DROP TRIGGER fail_qualification_update`)
	}
	fail()
	body := fmt.Sprintf(`{"name":"停用失败","description":"","enabled":false,"member_ids":[%q],"expected_version":%q}`, accountID(t, member), role.Version)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, admin, "PUT", "/api/v1/approval-roles/"+role.ID, body, "failure-role-disable"), 503, "approval_role_not_saved")
	current := decodeApprovalRole(t, releaseActorRequest(t, app, admin, "GET", "/api/v1/approval-roles/"+role.ID, "", ""))
	if !current.Enabled || current.Version != role.Version {
		t.Fatal("notification failure committed role update")
	}
	if output, err := f.command("", "disable", "--id", accountID(t, member)).CombinedOutput(); err == nil {
		t.Fatalf("notification failure committed account disable: %s", output)
	}
	assertApprovalCounts(t, app, member, 1, 1)
	assertApprovalCounts(t, app, fallback, 0, 0)
	restore()
	role = updateTableRole(t, app, role, "停用成功", false, member)
	assertApprovalCounts(t, app, fallback, 1, 1)
	fail()
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, admin, "PUT", "/api/v1/account-roles/"+accountID(t, fallback), `{"roles":["VIEWER"],"expected_version":"2"}`, "failure-demote"), 503, "auth_unavailable")
	assertApprovalCounts(t, app, fallback, 1, 1)
	restore()
	grantReleaseRole(t, app, fallback, `["VIEWER"]`, "2", "failure-demote")
	assertApprovalCounts(t, app, fallback, 0, 0)
	fail()
	if output, err := f.command("", "grant-admin", "--id", accountID(t, fallback)).CombinedOutput(); err == nil {
		t.Fatalf("notification failure committed maintenance grant: %s", output)
	}
	assertApprovalCounts(t, app, fallback, 0, 0)
	restore()
	f.run(t, "", "grant-admin", "--id", accountID(t, fallback))
	assertApprovalCounts(t, app, fallback, 1, 1)
}

// AC-018: the transport consumes and loses the real committed HTTP response.
// Original-body retry recovers one event and cannot advance notification progress.
func TestApprovalNotificationsLostHTTPResponsesRecoverOriginalFacts(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "lost.notice", "lost.notice@example.com", "correct horse battery staple")
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "响应丢失", member))
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	transport := &droppedResponseTransport{base: http.DefaultTransport}
	client := &http.Client{Transport: transport}
	for _, action := range []string{"submit", "approve", "reject", "cancel"} {
		t.Run(action, func(t *testing.T) {
			path := createTableApprovalDraft(t, app, admin, "lost-notice-"+action)
			actor := admin
			body := `{"expected_version":"1"}`
			if action != "submit" {
				rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", body, "lost-pre-submit-"+action), 200)
				if action == "cancel" {
					body = `{"expected_version":"2","reason":"响应丢失取消"}`
				} else {
					actor = member
					body = confirmedApprovalBody(t, app, member, path, "响应丢失决定")
				}
			}
			request, err := http.NewRequest("POST", server.URL+path+"/"+action, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			for _, cookie := range actor.Result().Cookies() {
				request.AddCookie(cookie)
			}
			request.Header.Set("Origin", "http://127.0.0.1:5173")
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", sessionCSRF(t, actor))
			request.Header.Set("Idempotency-Key", "lost-action-"+action)
			transport.drop.Store(true)
			response, err := client.Do(request)
			if err == nil || response != nil {
				t.Fatalf("expected response loss: %v %v", response, err)
			}
			before := readTableApprovalOrder(t, app, admin, path)
			applicant := readApprovalProgress(t, app, admin, path)
			reviewer := readApprovalProgress(t, app, member, path)
			rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", path+"/"+action, body, "lost-action-"+action), 200)
			after := readTableApprovalOrder(t, app, admin, path)
			if after.Version != before.Version || len(after.History) != len(before.History) {
				t.Fatal("response loss replayed business event")
			}
			if readApprovalProgress(t, app, admin, path) != applicant || readApprovalProgress(t, app, member, path) != reviewer {
				t.Fatal("response loss duplicated a notification")
			}
		})
	}
}

func TestApprovalNotificationsConcurrentLateReadPreservesNewEvents(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "concurrent.notice", "concurrent.notice@example.com", "correct horse battery staple")
	role := tableApprovalRole(t, app, "并发提醒", member)
	assignTableApproval(t, app, "mutation_add_items", role)
	path := createTableApprovalDraft(t, app, admin, "concurrent-notice")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "concurrent-submit"), 200)
	old := readApprovalProgress(t, app, member, path)
	role = updateTableRole(t, app, role, "资格暂退", false, member)
	type concurrentRequest struct {
		actor                   *httptest.ResponseRecorder
		method, path, body, key string
	}
	race := func(requests []concurrentRequest) {
		start := make(chan struct{})
		responses := make(chan *httptest.ResponseRecorder, len(requests))
		for _, request := range requests {
			cookies, csrf := request.actor.Result().Cookies(), sessionCSRF(t, request.actor)
			go func() {
				<-start
				responses <- accountRequestFrom(app, request.method, request.path, request.body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": request.key})
			}()
		}
		close(start)
		for range requests {
			response := <-responses
			if response.Code != 200 {
				t.Fatalf("concurrent update: %d %s", response.Code, response.Body)
			}
		}
	}
	race([]concurrentRequest{
		{actor: admin, method: "PUT", path: "/api/v1/approval-roles/" + role.ID, body: fmt.Sprintf(`{"name":"资格恢复","description":"","enabled":true,"member_ids":[%q],"expected_version":%q}`, accountID(t, member), role.Version), key: "concurrent-restore"},
		{actor: member, method: "POST", path: path + "/notification-read", body: fmt.Sprintf(`{"sequence":%q}`, old.Sequence)},
	})
	assertApprovalCounts(t, app, member, 1, 1)
	latest := readApprovalProgress(t, app, member, path)
	if latest.Sequence != "2" {
		t.Fatalf("new qualification lost: %+v", latest)
	}
	applicant := readApprovalProgress(t, app, admin, path)
	race([]concurrentRequest{
		{actor: member, method: "POST", path: path + "/approve", body: confirmedApprovalBody(t, app, member, path, "并发批准"), key: "concurrent-approve"},
		{actor: admin, method: "POST", path: path + "/notification-read", body: fmt.Sprintf(`{"sequence":%q}`, applicant.Sequence)},
	})
	assertApprovalCounts(t, app, admin, 1, 0)
	assertApprovalCounts(t, app, member, 0, 0)
}

func TestApprovalNotificationsOwnActionsPreserveEarlierResultsAndTruthfulPending(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	editor := registerAccount(t, app, "self.editor", "self.editor@example.com", "correct horse battery staple")
	goods := registerAccount(t, app, "self.goods", "self.goods@example.com", "correct horse battery staple")
	prices := registerAccount(t, app, "self.prices", "self.prices@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "1", "self-editor")
	role := tableApprovalRole(t, app, "本人加入", goods)
	assignTableApproval(t, app, "mutation_add_items", role)
	assignTableApproval(t, app, "mutation_supplied_id_items", tableApprovalRole(t, app, "价格待审", prices))
	body := `{"title":"保留他人未读进展","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":"self-change","label":"intent"}},{"table_name":"mutation_supplied_id_items","operation":"ADD","content":{"id":"self-change","label":"intent"}}]}`
	order := rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", "/api/v1/release-orders", body, "self-create"), 201)
	path := "/api/v1/release-orders/" + order.ID
	rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/submit", `{"expected_version":"1"}`, "self-submit"), 200)
	// The administrator adds themself: truthful pending, but no new own reminder.
	role = updateTableRole(t, app, role, "本人加入", true, goods, admin)
	assertApprovalCounts(t, app, admin, 0, 1)
	assertApprovalCounts(t, app, goods, 1, 1)
	assertNotificationOrders(t, readNotificationPage(t, app, admin, "view=pending&unread=true"))
	rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", confirmedApprovalBody(t, app, goods, path, "他人进展"), "self-goods-approve"), 200)
	assertApprovalCounts(t, app, editor, 1, 0)
	previous := readApprovalProgress(t, app, editor, path)
	rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/cancel", `{"expected_version":"3","reason":"本人结束申请"}`, "self-cancel"), 200)
	if next := readApprovalProgress(t, app, editor, path); next != previous {
		t.Fatalf("own cancellation overwrote prior unread outcome: %+v -> %+v", previous, next)
	}
	assertApprovalCounts(t, app, prices, 0, 0)
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=handled"), path)
	// Cancellation by another administrator does notify the applicant.
	path = createTableApprovalDraft(t, app, editor, "other-cancel")
	rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/submit", `{"expected_version":"1"}`, "other-submit"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/cancel", `{"expected_version":"2","reason":"管理员取消"}`, "other-cancel"), 200)
	assertApprovalCounts(t, app, editor, 2, 0)
}
