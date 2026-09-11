//go:build integration

package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// AC-008: a derived draft confirms the exact ordered source details. The
// caller may refresh record versions, but may not replace a stable detail ID.
func TestMultitableDerivedDraftRequiresExactDetailIdentity(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	db := deliveryDB(t, driver)
	for _, table := range []string{"derived_identity_a", "derived_identity_b"} {
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY, label VARCHAR(80))")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true})
	}
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"stable multitable details","items":[{"table_name":"derived_identity_a","operation":"ADD","content":{"id":"1","label":"A"}},{"table_name":"derived_identity_b","operation":"ADD","content":{"id":"1","label":"B"}}]}`, "derived-identity-create"), 201)
	path := "/api/v1/release-orders/" + original.ID
	cancelled := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"1","reason":"copy with current baselines"}`, "derived-identity-cancel"), 200)
	items := []map[string]any{
		{"detail_id": cancelled.Items[0].DetailID, "table_name": cancelled.Items[0].TableName, "operation": cancelled.Items[0].Operation, "expected_record_version": cancelled.Items[0].ExpectedRecordVersion, "content": cancelled.Items[0].Content},
		{"detail_id": "ffffffffffffffffffffffffffffffff", "table_name": cancelled.Items[1].TableName, "operation": cancelled.Items[1].Operation, "expected_record_version": cancelled.Items[1].ExpectedRecordVersion, "content": cancelled.Items[1].Content},
	}
	body, err := json.Marshal(map[string]any{"expected_version": cancelled.Version, "confirmed": true, "items": items})
	if err != nil {
		t.Fatal(err)
	}
	response := releaseRequest(t, app, "POST", path+"/copy", string(body), "derived-identity-copy")
	assertIntegrationErrorCode(t, response, 422, "release_invalid")

	items[1]["detail_id"] = cancelled.Items[1].DetailID
	items[1]["table_name"] = cancelled.Items[0].TableName
	body, err = json.Marshal(map[string]any{"expected_version": cancelled.Version, "confirmed": true, "items": items})
	if err != nil {
		t.Fatal(err)
	}
	crossTable := releaseRequest(t, app, "POST", path+"/copy", string(body), "derived-identity-table")
	assertIntegrationErrorCode(t, crossTable, 422, "release_cross_table")
	if !strings.Contains(crossTable.Body.String(), `"item_index":1`) {
		t.Fatalf("table identity mismatch is not located: %s", crossTable.Body)
	}

	items = derivedDraftItems(cancelled)
	for _, item := range items {
		delete(item, "detail_id")
	}
	body, err = json.Marshal(map[string]any{"expected_version": cancelled.Version, "confirmed": true, "items": items})
	if err != nil {
		t.Fatal(err)
	}
	derived := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/copy", string(body), "derived-identity-omitted"), 201)
	if derived.Items[0].DetailID != cancelled.Items[0].DetailID || derived.Items[1].DetailID != cancelled.Items[1].DetailID || derived.Items[0].TableName != cancelled.Items[0].TableName || derived.Items[1].TableName != cancelled.Items[1].TableName {
		t.Fatalf("omitted detail identities were not restored at their original table/position: %+v", derived.Items)
	}
}

// AC-008: a target conflict in a later table rejects the whole copy, identifies
// its owner, and leaves no first-table reservation or half-created order. Once
// the conflict ends, the same request creates one independently owned draft and
// links both the source and derivative histories.
func TestMultitableCopyCompetesForEveryTargetAndLinksBothOrders(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	db := deliveryDB(t, driver)
	for _, table := range []string{"derived_copy_a", "derived_copy_b"} {
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY, code VARCHAR(80))")
		deliveryExec(t, db, "INSERT INTO "+table+" VALUES(1,'old-"+table[len(table)-1:]+"')")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowModify: true})
		setDraftTestKey(t, app, table, []string{"code"})
	}
	owner := registerAccount(t, app, "copy.source", "copy.source@example.com", "correct horse battery staple")
	copier := registerAccount(t, app, "copy.operator", "copy.operator@example.com", "correct horse battery staple")
	blocker := registerAccount(t, app, "copy.blocker", "copy.blocker@example.com", "correct horse battery staple")
	for index, account := range []*httptest.ResponseRecorder{owner, copier, blocker} {
		grantReleaseRole(t, app, account, `["EDITOR"]`, "1", fmt.Sprintf("copy-role-%d", index))
	}

	source := rollbackOrderResponse(t, releaseActorRequest(t, app, owner, "POST", "/api/v1/release-orders", `{"title":"copy every table","items":[{"table_name":"derived_copy_a","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"next-a"}},{"table_name":"derived_copy_b","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"next-b"}}]}`, "copy-multi-source"), 201)
	sourcePath := "/api/v1/release-orders/" + source.ID
	source = rollbackOrderResponse(t, releaseActorRequest(t, app, owner, "POST", sourcePath+"/cancel", `{"expected_version":"1","reason":"refresh both tables"}`, "copy-multi-source-cancel"), 200)
	deliveryExec(t, db, `UPDATE derived_copy_b SET code='drift-b' WHERE id=1`)
	occupied := rollbackOrderResponse(t, releaseActorRequest(t, app, blocker, "POST", "/api/v1/release-orders", `{"title":"later table owner","items":[{"table_name":"derived_copy_b","operation":"ADD","content":{"id":"99","code":"drift-b"}}]}`, "copy-multi-blocker"), 201)
	body := derivedDraftBody(t, source)
	conflict := releaseActorRequest(t, app, copier, "POST", sourcePath+"/copy", body, "copy-multi-apply")
	assertIntegrationErrorCode(t, conflict, 409, "release_target_conflict")
	if !strings.Contains(conflict.Body.String(), `"item_index":1`) || !strings.Contains(conflict.Body.String(), `"table_name":"derived_copy_b"`) || !strings.Contains(conflict.Body.String(), `"order_id":"`+occupied.ID+`"`) || !strings.Contains(conflict.Body.String(), `"applicant_id":"`+accountID(t, blocker)+`"`) {
		t.Fatalf("later-table conflict is not located: %s", conflict.Body)
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM rcc_release_orders`:                                                2,
		`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + occupied.ID + `'`:          2,
		`SELECT COUNT(*) FROM rcc_release_targets WHERE table_name='derived_copy_a'`:             0,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'copy:%'`:                0,
		`SELECT COUNT(*) FROM rcc_release_table_references WHERE order_id='` + occupied.ID + `'`: 1,
	})

	rollbackOrderResponse(t, releaseActorRequest(t, app, blocker, "POST", "/api/v1/release-orders/"+occupied.ID+"/cancel", `{"expected_version":"1","reason":"release later target"}`, "copy-multi-blocker-cancel"), 200)
	copiedResponse := releaseActorRequest(t, app, copier, "POST", sourcePath+"/copy", body, "copy-multi-apply")
	copied := rollbackOrderResponse(t, copiedResponse, 201)
	if copied.Title != source.Title || copied.ApplicantID != accountID(t, copier) || copied.CopiedFromID != source.ID || len(copied.Items) != 2 || copied.Items[0].TableName != "derived_copy_a" || copied.Items[1].TableName != "derived_copy_b" || copied.Items[0].DetailID != source.Items[0].DetailID || copied.Items[1].DetailID != source.Items[1].DetailID || copied.Items[0].Operation != source.Items[0].Operation || copied.Items[1].Operation != source.Items[1].Operation || !reflect.DeepEqual(copied.Items[0].Content, source.Items[0].Content) || !reflect.DeepEqual(copied.Items[1].Content, source.Items[1].Content) || *copied.Items[1].Before["code"] != "drift-b" {
		t.Fatalf("incomplete copied draft: %+v", copied)
	}
	replay := releaseActorRequest(t, app, copier, "POST", sourcePath+"/copy", body, "copy-multi-apply")
	if replay.Code != 201 || replay.Body.String() != copiedResponse.Body.String() {
		t.Fatalf("copy replay changed result: %d %s", replay.Code, replay.Body)
	}
	currentSource := rollbackOrderResponse(t, releaseActorReadAllDetails(t, app, copier, "GET", sourcePath, "", ""), 200)
	last := currentSource.History[len(currentSource.History)-1]
	if currentSource.State != "CANCELLED" || last.Action != "COPY" || last.RelatedOrderID != copied.ID {
		t.Fatalf("source lacks reverse copy relation: %+v", currentSource.History)
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM rcc_release_orders`:                                              3,
		`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + copied.ID + `'`:          6,
		`SELECT COUNT(*) FROM rcc_release_table_references WHERE order_id='` + copied.ID + `'`: 2,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'copy:%'`:              1,
	})
}

func derivedDraftBody(t *testing.T, source domain.ReleaseOrder) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"expected_version": source.Version, "confirmed": true, "items": derivedDraftItems(source)})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// AC-008: reprepare reads every current table baseline, then atomically retires
// the approval, creates its replacement, transfers retained targets and takes
// newly discovered targets. A failure at the final table-reference write keeps
// the approved source, its independent approval and every old reservation.
func TestMultitableReprepareTransfersChangedTargetsAtomically(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	db := deliveryDB(t, driver)
	ownerDriver := *driver
	ownerDriver.User = "root"
	ownerDB := deliveryDB(t, &ownerDriver)
	for _, table := range []string{"reprepare_multi_a", "reprepare_multi_b"} {
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY, code VARCHAR(80))")
		deliveryExec(t, db, "INSERT INTO "+table+" VALUES(1,'old-"+table[len(table)-1:]+"')")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowModify: true})
		setDraftTestKey(t, app, table, []string{"code"})
	}
	applicant := registerAccount(t, app, "reprepare.multi.owner", "reprepare.multi.owner@example.com", "correct horse battery staple")
	reviewer := registerAccount(t, app, "reprepare.multi.reviewer", "reprepare.multi.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, applicant, `["EDITOR"]`, "1", "reprepare-multi-owner-role")
	configurePublicationReviewer(t, app, reviewer, "reprepare_multi_a", "reprepare_multi_b")
	admin := integrationAdminSession(t, app)

	source := rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", `{"title":"reprepare current multitable baselines","items":[{"table_name":"reprepare_multi_a","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"next-a"}},{"table_name":"reprepare_multi_b","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"next-b"}}]}`, "reprepare-multi-create"), 201)
	path := "/api/v1/release-orders/" + source.ID
	source = rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1"}`, "reprepare-multi-submit"), 200)
	source = rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, reviewer, path, "independent multitable approval"), "reprepare-multi-approve"), 200)
	deliveryExec(t, db, `UPDATE reprepare_multi_b SET code='drift-b' WHERE id=1`)
	previewInput := map[string]any{"title": source.Title, "items": derivedDraftItems(source)}
	previewBody, _ := json.Marshal(previewInput)
	previewResponse := releaseActorRequest(t, app, admin, "POST", "/api/v1/release-orders/preview", string(previewBody), "")
	var preview struct {
		Items []domain.ReleaseItem `json:"items"`
	}
	if previewResponse.Code != 200 || json.Unmarshal(previewResponse.Body.Bytes(), &preview) != nil || len(preview.Items) != 2 || *preview.Items[1].Before["code"] != "drift-b" {
		t.Fatalf("fresh multitable preview: %d %s", previewResponse.Code, previewResponse.Body)
	}
	confirmed := source
	confirmed.Items = preview.Items
	body := derivedDraftBody(t, confirmed)
	approved := releaseActorReadAllDetails(t, app, admin, "GET", path, "", "")

	deliveryExec(t, ownerDB, `CREATE TRIGGER reject_reprepared_reference BEFORE INSERT ON rcc_release_table_references FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected reprepare reference failure'`)
	failed := releaseActorRequest(t, app, admin, "POST", path+"/reprepare", body, "reprepare-multi-apply")
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	unchanged := releaseActorReadAllDetails(t, app, admin, "GET", path, "", "")
	if unchanged.Body.String() != approved.Body.String() {
		t.Fatalf("late failure changed approved source: %s", unchanged.Body)
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM rcc_release_orders`:                                              1,
		`SELECT COUNT(*) FROM rcc_release_details WHERE order_id='` + source.ID + `'`:          2,
		`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + source.ID + `'`:          6,
		`SELECT COUNT(*) FROM rcc_release_table_references WHERE order_id='` + source.ID + `'`: 2,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'reprepare:%'`:         0,
	})
	deliveryExec(t, ownerDB, `DROP TRIGGER reject_reprepared_reference`)

	// Hold the reprepare after its DELETE of a target retained by the replacement.
	// A third order races for that exact target while the transaction is open. It
	// must wait and then report the replacement as owner, never acquire a target
	// exposed between the source release and replacement reservation.
	gate := "rcc_issue85_reprepare_gate"
	entered := "rcc_issue85_reprepare_entered"
	gateConn, err := ownerDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gateConn.Close()
	var locked int
	if err := gateConn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 5)`, gate).Scan(&locked); err != nil || locked != 1 {
		t.Fatalf("hold reprepare gate: locked=%d err=%v", locked, err)
	}
	schema, err := app.mysql.GetTableSchema(ctx, "reprepare_multi_b")
	if err != nil {
		t.Fatal(err)
	}
	var retainedKey []byte
	if err := app.mysql.ExecuteReleaseOrder(ctx, func(session application.ReleaseOrderSession) error {
		keys, readErr := session.ReadConcurrencyKeys(ctx, schema, []string{"code"}, []domain.Row{{"code": source.Items[1].Content["code"]}})
		if readErr == nil {
			retainedKey = keys[0]
		}
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, ownerDB, fmt.Sprintf(`CREATE TRIGGER pause_reprepare_transfer AFTER DELETE ON rcc_release_targets FOR EACH ROW BEGIN IF OLD.order_id='%s' AND OLD.table_name='reprepare_multi_b' AND HEX(OLD.record_key)='%s' THEN SET @rcc_issue85_entered=GET_LOCK('%s',0); SET @rcc_issue85_gate=GET_LOCK('%s',10); END IF; END`, source.ID, strings.ToUpper(hex.EncodeToString(retainedKey)), entered, gate))
	defer deliveryExec(t, ownerDB, `DROP TRIGGER IF EXISTS pause_reprepare_transfer`)

	adminCSRF, adminCookies := sessionCSRF(t, admin), admin.Result().Cookies()
	repreparedResponses := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		repreparedResponses <- accountRequestFrom(app, "POST", path+"/reprepare", body, adminCookies, adminCSRF, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "reprepare-multi-apply"})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var holder sql.NullInt64
		if err := ownerDB.QueryRowContext(ctx, `SELECT IS_USED_LOCK(?)`, entered).Scan(&holder); err != nil {
			t.Fatal(err)
		}
		if holder.Valid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reprepare did not reach target transfer gate")
		}
		time.Sleep(10 * time.Millisecond)
	}
	contenderBody := `{"title":"race retained target","items":[{"table_name":"reprepare_multi_b","operation":"ADD","content":{"id":"101","code":"next-b"}}]}`
	applicantCSRF, applicantCookies := sessionCSRF(t, applicant), applicant.Result().Cookies()
	contenderResponses := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		contenderResponses <- accountRequestFrom(app, "POST", "/api/v1/release-orders", contenderBody, applicantCookies, applicantCSRF, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "reprepare-multi-contender"})
	}()
	deadline = time.Now().Add(3 * time.Second)
	// Release writes serialize current authorization before reading targets. The
	// contender must wait there until the transfer transaction publishes its new owner.
	for {
		var waiting int
		if err := ownerDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks requested ON requested.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID JOIN performance_schema.data_locks blocking ON blocking.ENGINE_LOCK_ID=w.BLOCKING_ENGINE_LOCK_ID WHERE requested.OBJECT_SCHEMA=DATABASE() AND requested.OBJECT_NAME='rcc_auth_control_lock' AND requested.INDEX_NAME='PRIMARY' AND blocking.OBJECT_SCHEMA=requested.OBJECT_SCHEMA AND blocking.OBJECT_NAME=requested.OBJECT_NAME`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case early := <-contenderResponses:
			t.Fatalf("contender escaped open transfer transaction: %d %s", early.Code, early.Body)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("contender did not reach the release authorization lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := gateConn.QueryRowContext(ctx, `SELECT RELEASE_LOCK(?)`, gate).Scan(&locked); err != nil || locked != 1 {
		t.Fatalf("release reprepare gate: locked=%d err=%v", locked, err)
	}
	var repreparedResponse *httptest.ResponseRecorder
	select {
	case repreparedResponse = <-repreparedResponses:
	case <-time.After(5 * time.Second):
		t.Fatal("reprepare did not complete after transfer gate opened")
	}
	reprepared := rollbackOrderResponse(t, repreparedResponse, 201)
	var contender *httptest.ResponseRecorder
	select {
	case contender = <-contenderResponses:
	case <-time.After(5 * time.Second):
		t.Fatal("contender remained blocked after reprepare committed")
	}
	assertIntegrationErrorCode(t, contender, 409, "release_target_conflict")
	if !strings.Contains(contender.Body.String(), `"order_id":"`+reprepared.ID+`"`) {
		t.Fatalf("transfer contender did not resolve replacement owner: %s", contender.Body)
	}
	if reprepared.Title != source.Title || reprepared.ApplicantID != accountID(t, admin) || reprepared.CopiedFromID != source.ID || reprepared.State != "DRAFT" || len(reprepared.Items) != 2 || *reprepared.Items[1].Before["code"] != "drift-b" || reprepared.Items[0].DetailID != source.Items[0].DetailID || reprepared.Items[1].DetailID != source.Items[1].DetailID || reprepared.Items[0].Operation != source.Items[0].Operation || reprepared.Items[1].Operation != source.Items[1].Operation || !reflect.DeepEqual(reprepared.Items[0].Content, source.Items[0].Content) || !reflect.DeepEqual(reprepared.Items[1].Content, source.Items[1].Content) {
		t.Fatalf("invalid replacement draft: %+v", reprepared)
	}
	replay := releaseActorRequest(t, app, admin, "POST", path+"/reprepare", body, "reprepare-multi-apply")
	if replay.Code != 201 || replay.Body.String() != repreparedResponse.Body.String() {
		t.Fatalf("reprepare replay changed result: %d %s", replay.Code, replay.Body)
	}
	retired := rollbackOrderResponse(t, releaseActorReadAllDetails(t, app, admin, "GET", path, "", ""), 200)
	last := retired.History[len(retired.History)-1]
	if retired.State != "CANCELLED" || last.Action != "REPREPARE" || last.RelatedOrderID != reprepared.ID {
		t.Fatalf("source replacement relation: %+v", retired.History)
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM rcc_release_orders`:                                                  2,
		`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + source.ID + `'`:              0,
		`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + reprepared.ID + `'`:          6,
		`SELECT COUNT(*) FROM rcc_release_table_references WHERE order_id='` + source.ID + `'`:     0,
		`SELECT COUNT(*) FROM rcc_release_table_references WHERE order_id='` + reprepared.ID + `'`: 2,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'reprepare:%'`:             1,
	})

	draftPath := "/api/v1/release-orders/" + reprepared.ID
	submitted := rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", draftPath+"/submit", `{"expected_version":"1"}`, "reprepare-multi-resubmit"), 200)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, admin, "POST", draftPath+"/approve", `{"expected_version":"2","reason":"self approval must not carry"}`, "reprepare-multi-self-approve"), 403, "permission_denied")
	approvedReplacement := rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", draftPath+"/approve", confirmedApprovalBody(t, app, reviewer, draftPath, "fresh independent review"), "reprepare-multi-independent-approve"), 200)
	if approvedReplacement.State != "APPROVED" || submitted.State != "PENDING_APPROVAL" {
		t.Fatal("replacement did not require fresh approval")
	}

	oldKey := releaseActorRequest(t, app, admin, "POST", "/api/v1/release-orders", `{"title":"released old key","items":[{"table_name":"reprepare_multi_b","operation":"ADD","content":{"id":"99","code":"old-b"}}]}`, "reprepare-multi-old-key")
	rollbackOrderResponse(t, oldKey, 201)
	newKey := releaseActorRequest(t, app, admin, "POST", "/api/v1/release-orders", `{"title":"protected refreshed key","items":[{"table_name":"reprepare_multi_b","operation":"ADD","content":{"id":"100","code":"drift-b"}}]}`, "reprepare-multi-new-key")
	assertIntegrationErrorCode(t, newKey, 409, "release_target_conflict")
	if !strings.Contains(newKey.Body.String(), `"order_id":"`+reprepared.ID+`"`) {
		t.Fatalf("refreshed baseline target owner missing: %s", newKey.Body)
	}
}

func derivedDraftItems(source domain.ReleaseOrder) []map[string]any {
	items := make([]map[string]any, len(source.Items))
	for index, item := range source.Items {
		items[index] = map[string]any{"detail_id": item.DetailID, "table_name": item.TableName, "operation": item.Operation, "expected_record_version": item.ExpectedRecordVersion, "content": item.Content}
		if item.Operation != "ADD" {
			items[index]["id"] = item.ID
		}
	}
	return items
}
