//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type notificationPage struct {
	Orders []domain.ReleaseOrderSummary `json:"orders"`
	Next   string                       `json:"next_cursor"`
}

func readNotificationPage(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, query string) notificationPage {
	t.Helper()
	response := releaseActorRequest(t, app, actor, "GET", "/api/v1/release-orders?"+query, "", "")
	if response.Code != 200 {
		t.Fatalf("notification list: %d %s", response.Code, response.Body)
	}
	var page notificationPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func assertNotificationOrders(t *testing.T, page notificationPage, paths ...string) {
	t.Helper()
	actual, expected := []string{}, []string{}
	for _, order := range page.Orders {
		actual = append(actual, order.ID)
	}
	for _, path := range paths {
		expected = append(expected, strings.TrimPrefix(path, "/api/v1/release-orders/"))
	}
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Fatalf("notification orders: got %v, want %v", actual, expected)
	}
}

// AC-012: submission is a durable fact, not simply a non-DRAFT current state.
func TestNotificationCenterSubmittedViewsExcludeUnsubmittedDrafts(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	reviewer := registerAccount(t, app, "center.reviewer", "center.reviewer@example.com", "correct horse battery staple")
	other := registerAccount(t, app, "center.other", "center.other@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, other, `["EDITOR"]`, "1", "center-other-editor")
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "通知审批", reviewer))
	createTableApprovalDraft(t, app, admin, "center-draft")
	cancelledDraft := createTableApprovalDraft(t, app, admin, "center-cancelled-draft")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", cancelledDraft+"/cancel", `{"expected_version":"1","reason":"从未提交"}`, "center-cancel-draft"), 200)
	submitted := createTableApprovalDraft(t, app, admin, "center-submitted")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", submitted+"/submit", `{"expected_version":"1"}`, "center-submit"), 200)
	cancelled := createTableApprovalDraft(t, app, admin, "center-cancelled")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", cancelled+"/submit", `{"expected_version":"1"}`, "center-submit-cancelled"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", cancelled+"/cancel", `{"expected_version":"2","reason":"已提交后取消"}`, "center-cancel-submitted"), 200)
	otherSubmitted := createTableApprovalDraft(t, app, other, "center-other")
	rollbackOrderResponse(t, releaseActorRequest(t, app, other, "POST", otherSubmitted+"/submit", `{"expected_version":"1"}`, "center-other-submit"), 200)
	assertNotificationOrders(t, readNotificationPage(t, app, reviewer, "view=all"), submitted, cancelled, otherSubmitted)
	assertNotificationOrders(t, readNotificationPage(t, app, admin, "view=mine"), submitted, cancelled)
	assertNotificationOrders(t, readNotificationPage(t, app, other, "view=mine"), otherSubmitted)
	assertNotificationOrders(t, readNotificationPage(t, app, reviewer, "view=handled"))
	assertNotificationOrders(t, readNotificationPage(t, app, reviewer, "view=pending"), submitted, otherSubmitted)
	assertNotificationOrders(t, readNotificationPage(t, app, admin, "view=pending"))
}

// AC-012: multiple tables/roles and repeated decisions still have one list row.
func TestNotificationCenterCurrentEligibilityAndDurableHandledHistory(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	goods := registerAccount(t, app, "center.goods", "center.goods@example.com", "correct horse battery staple")
	prices := registerAccount(t, app, "center.prices", "center.prices@example.com", "correct horse battery staple")
	fallback := registerAccount(t, app, "center.fallback", "center.fallback@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, fallback, `["ADMIN"]`, "1", "center-fallback-admin")
	goodsRole := tableApprovalRole(t, app, "商品审批", goods)
	duplicateRole := tableApprovalRole(t, app, "第二商品审批", goods)
	priceRole := tableApprovalRole(t, app, "价格审批", prices)
	assignTableApproval(t, app, "mutation_add_items", goodsRole, duplicateRole)
	assignTableApproval(t, app, "mutation_supplied_id_items", priceRole)
	create := func(code string) string {
		body := fmt.Sprintf(`{"title":"通知中心多表审批","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":%q,"label":"intent"}},{"table_name":"mutation_supplied_id_items","operation":"ADD","content":{"id":%q,"label":"intent"}}]}`, code, code)
		created := rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", "/api/v1/release-orders", body, "center-create-"+code), 201)
		path := "/api/v1/release-orders/" + created.ID
		rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "center-submit-"+code), 200)
		return path
	}
	path := create("center-multi")
	page := readNotificationPage(t, app, goods, "view=pending")
	assertNotificationOrders(t, page, path)
	detail := readTableApprovalOrder(t, app, goods, path)
	if !slices.Equal(page.Orders[0].ApprovalContext.ApprovableTables, []string{"mutation_add_items"}) || page.Orders[0].ApprovalContext.Revision != detail.ApprovalContext.Revision {
		t.Fatal("list/detail current scope or revision diverged")
	}
	assertNotificationOrders(t, readNotificationPage(t, app, fallback, "view=all"), path)
	assertNotificationOrders(t, readNotificationPage(t, app, fallback, "view=pending"))
	rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", confirmedApprovalBody(t, app, goods, path, "商品已处理"), "center-goods-approve"), 200)
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=pending"))
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=handled"), path)
	assertNotificationOrders(t, readNotificationPage(t, app, prices, "view=pending"), path)
	assertNotificationOrders(t, readNotificationPage(t, app, prices, "view=handled"))
	// Removing all independent price members gives the fallback ADMIN the remainder.
	priceRole = updateTableRole(t, app, priceRole, "价格无人", true)
	assertNotificationOrders(t, readNotificationPage(t, app, prices, "view=pending"))
	assertNotificationOrders(t, readNotificationPage(t, app, fallback, "view=pending"), path)
	assertNotificationOrders(t, readNotificationPage(t, app, admin, "view=pending"))
	// The original reviewer joins the remaining table and really decides a second time.
	priceRole = updateTableRole(t, app, priceRole, "商品接手价格", true, goods)
	assertNotificationOrders(t, readNotificationPage(t, app, fallback, "view=pending"))
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=pending"), path)
	rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", confirmedApprovalBody(t, app, goods, path, "价格也已处理"), "center-price-approve"), 200)
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=handled"), path)
	goodsRole = updateTableRole(t, app, goodsRole, "商品撤权", false)
	updateTableRole(t, app, duplicateRole, "第二商品撤权", false)
	priceRole = updateTableRole(t, app, priceRole, "价格改派", true, prices)
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=handled"), path)
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=pending"))
	// Rejection is a real handled fact; merely being another eligible reviewer is not.
	goodsRole = updateTableRole(t, app, goodsRole, "商品恢复", true, goods)
	rejected := create("center-rejected")
	rollbackOrderResponse(t, releaseActorRequest(t, app, prices, "POST", rejected+"/reject", confirmedApprovalBody(t, app, prices, rejected, "价格拒绝"), "center-reject"), 200)
	updateTableRole(t, app, priceRole, "价格撤权", false)
	assertNotificationOrders(t, readNotificationPage(t, app, prices, "view=handled"), rejected)
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=handled"), path)
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=pending"))
	assertNotificationOrders(t, readNotificationPage(t, app, admin, "view=all&state=REJECTED"), rejected)
}

// AC-012: cursor pages must cross arbitrarily many unqualified candidates.
func TestNotificationCenterFiltersBeforePaginationAndProtectsIdentity(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "center.page", "center.page@example.com", "correct horse battery staple")
	other := registerAccount(t, app, "center.page.other", "center.page.other@example.com", "correct horse battery staple")
	paths := []string{}
	roles := map[string]approvalRoleResult{}
	for index := range 10 {
		code := fmt.Sprintf("center-page-%d", index)
		role := tableApprovalRole(t, app, code, other)
		assignTableApproval(t, app, "mutation_add_items", role)
		path := createTableApprovalDraft(t, app, admin, code)
		rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "submit-"+code), 200)
		paths = append(paths, path)
		roles[path] = role
	}
	slices.Sort(paths)
	// Make only the 7th and 10th IDs visible, after six full unqualified pages.
	for _, path := range []string{paths[6], paths[9]} {
		updateTableRole(t, app, roles[path], "分页接手", true, member)
	}
	first := readNotificationPage(t, app, member, "view=pending&limit=1")
	assertNotificationOrders(t, first, paths[6])
	if first.Next != first.Orders[0].ID {
		t.Fatalf("wrong visible cursor: %+v", first)
	}
	last := readNotificationPage(t, app, member, "view=pending&limit=1&after="+first.Next)
	assertNotificationOrders(t, last, paths[9])
	if last.Next != "" {
		t.Fatalf("last full page must not advertise an empty next page: %q", last.Next)
	}
	assertNotificationOrders(t, readNotificationPage(t, app, member, "view=pending&limit=1&id="+strings.TrimPrefix(paths[9], "/api/v1/release-orders/")), paths[9])
	assertNotificationOrders(t, readNotificationPage(t, app, member, "view=pending&state=APPROVED"))
	assertNotificationOrders(t, readNotificationPage(t, app, member, "view=all&table_name=unknown_table"))
	// Caller-provided filters cannot impersonate the subject of a personal view.
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, member, "GET", "/api/v1/release-orders?view=mine&applicant_id="+accountID(t, admin), "", ""), 422, "release_invalid")
	assertNotificationOrders(t, readNotificationPage(t, app, member, "view=handled&applicant_id="+accountID(t, admin)))
	for _, query := range []string{"view=", "view=unknown", "view=all&limit=101", "view=all&state=unknown"} {
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, member, "GET", "/api/v1/release-orders?"+query, "", ""), 422, "release_invalid")
	}
	unauthenticated := accountRequest(app, "GET", "/api/v1/release-orders?view=all", "", nil, "")
	if unauthenticated.Code != 401 {
		t.Fatalf("unprotected center: %d %s", unauthenticated.Code, unauthenticated.Body)
	}
}

// Incompatible old orders and unavailable directories are real read failures,
// not an empty successful list or an invented default approval assignment.
func TestNotificationCenterRejectsIncompatibleHistoryAndDirectoryFailure(t *testing.T) {
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
	member := registerAccount(t, app, "center.history", "center.history@example.com", "correct horse battery staple")
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "历史审批", member))
	path := createTableApprovalDraft(t, app, admin, "center-history")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "center-history-submit"), 200)
	original := readTableApprovalOrder(t, app, admin, path)
	// Database boundary fixture representing a pre-table-approval document.
	id := strings.TrimPrefix(path, "/api/v1/release-orders/")
	deliveryExec(t, owner, `UPDATE rcc_release_orders SET document=JSON_REMOVE(document,'$.approvals') WHERE id=?`, id)
	for _, view := range []string{"mine", "all", "pending"} {
		failed := releaseActorRequest(t, app, admin, "GET", "/api/v1/release-orders?view="+view, "", "")
		assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	}
	approvals, err := json.Marshal(original.Approvals)
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, `UPDATE rcc_release_orders SET document=JSON_SET(document,'$.approvals',CAST(? AS JSON)) WHERE id=?`, string(approvals), id)
	current := createTableApprovalDraft(t, app, admin, "center-directory-failure")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", current+"/submit", `{"expected_version":"1"}`, "center-directory-submit"), 200)
	deliveryExec(t, owner, `RENAME TABLE rcc_approval_role_members TO unavailable_center_members`)
	failed := releaseActorRequest(t, app, member, "GET", "/api/v1/release-orders?view=pending", "", "")
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	deliveryExec(t, owner, `RENAME TABLE unavailable_center_members TO rcc_approval_role_members`)
	assertNotificationOrders(t, readNotificationPage(t, app, member, "view=pending"), path, current)
}
