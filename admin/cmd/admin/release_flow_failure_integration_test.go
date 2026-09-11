//go:build integration

package main

import (
	"reflect"
	"testing"
)

// Save owns definitions, order revision, intent and original request result in
// one transaction. Read failures never mean missing configuration or a default.
func TestReleaseFlowStorageFailuresKeepDraftAndOriginalResultAtomic(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/003-policy-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	saved := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "flow-failure-create"), 201)
	path := "/api/v1/release-orders/" + saved.ID
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	body := `{"title":"显式补齐流程","expected_version":"1","changes":{"upserts":[],"delete_detail_ids":[]}}`
	deliveryExec(t, db, `RENAME TABLE rcc_release_templates TO unavailable_release_templates`)
	failed := releaseRequest(t, app, "PUT", path, body, "flow-storage-repair")
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	read := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(read.TableFlows, saved.TableFlows) || !reflect.DeepEqual(read.MissingFlowTables, saved.MissingFlowTables) || read.Version != "1" || read.Title != saved.Title {
		t.Fatal("failed read changed acknowledged draft", read)
	}
	deliveryExec(t, db, `RENAME TABLE unavailable_release_templates TO rcc_release_templates`)
	deliveryExec(t, db, `CREATE TRIGGER reject_flow_save BEFORE UPDATE ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected instance storage failure'`)
	failed = releaseRequest(t, app, "PUT", path, body, "flow-storage-repair")
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	deliveryExec(t, db, `DROP TRIGGER reject_flow_save`)
	deliveryExec(t, db, `CREATE TRIGGER reject_flow_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'edit:%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected original result failure'; END IF; END`)
	failed = releaseRequest(t, app, "PUT", path, body, "flow-storage-repair")
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	read = flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(read.TableFlows, saved.TableFlows) || !reflect.DeepEqual(read.MissingFlowTables, saved.MissingFlowTables) || read.Version != "1" || read.Title != saved.Title {
		t.Fatal("failed original result committed partial draft", read)
	}
	deliveryExec(t, db, `DROP TRIGGER reject_flow_result`)
	repaired := flowResponse(t, releaseRequest(t, app, "PUT", path, body, "flow-storage-repair"), 200)
	if len(repaired.TableFlows) != 2 || len(repaired.MissingFlowTables) != 0 || repaired.Version != "2" || !reflect.DeepEqual(repaired.TableFlows[0], saved.TableFlows[0]) {
		t.Fatal("manual original save did not commit exactly once", repaired)
	}
	// GET must stay read-only even when the dependency rejects every order write.
	deliveryExec(t, db, `CREATE TRIGGER reject_flow_read_write BEFORE UPDATE ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='detail reads must not save'`)
	reread := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(reread.TableFlows, repaired.TableFlows) {
		t.Fatal("read changed stored nodes", reread)
	}
	replay := flowResponse(t, releaseRequest(t, app, "PUT", path, body, "flow-storage-repair"), 200)
	if !reflect.DeepEqual(replay.TableFlows, repaired.TableFlows) || replay.Version != "2" {
		t.Fatal("original request reran instance writes", replay)
	}
	deliveryExec(t, db, `DROP TRIGGER reject_flow_read_write`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, `{"title":"different","expected_version":"2","changes":{"upserts":[],"delete_detail_ids":[]}}`, "flow-storage-repair"), 409, "idempotency_conflict")
}
