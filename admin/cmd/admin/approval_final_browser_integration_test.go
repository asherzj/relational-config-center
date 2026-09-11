//go:build integration && browser

package main

import (
	"testing"

	passwordadapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
)

// AC-021/023 starts with a genuine v8 account and historical grant, performs
// the formal cutover, then exercises the complete current Web workflow.
func TestApprovalFinalBrowserSystemPath(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, buildSchemaMigrationReleaseAt(t, 8), driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	hash, err := passwordadapter.NewArgon2id().Hash(ctx, "legacy browser password long enough")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('00000000-0000-4000-8000-000000000198','legacy.browser','legacy.browser@example.test','旧审批人员 · 商品负责人',?,4,3,'2026-01-01')`, hash)
	deliveryExec(t, db, `INSERT INTO rcc_account_role_history(actor_kind,actor_id,account_id,before_roles,after_roles,version,request_key,request_digest,result,created_at) VALUES('maintenance','','00000000-0000-4000-8000-000000000198',1,4,3,'legacy-browser-grant',REPEAT('a',64),'{"Roles":4,"RoleVersion":3}','2026-01-01')`)
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), driver, "current", "up")
	initializeCurrentIntegrationSchema(t, ctx, driver, localManagedTableFixture, "testdata/016-multitable-browser.sql")
	runNotificationBrowserWithMySQL(t, driver, "approval-final.cjs")
}
