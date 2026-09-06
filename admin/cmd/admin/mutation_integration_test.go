//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

func TestRelationalMutationPolicyExecutesAuthorizationAutoFillAndOperationsInOneSnapshot(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/008-mutation-policy-snapshot-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	assignRelationalMutationPolicy(t, app, "snapshot_full_mutation_v1", true, true, true, true)

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open verification database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	var before time.Time
	if err := database.QueryRowContext(ctx, "SELECT UTC_TIMESTAMP(6)").Scan(&before); err != nil {
		t.Fatalf("read database time before ADD: %v", err)
	}
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"full-lifecycle","label":"created"}}`)
	id := mutationResponseID(t, added)
	var after, createdAt, updatedAt time.Time
	var creator, modifier, label string
	if err := database.QueryRowContext(ctx, `SELECT UTC_TIMESTAMP(6), creator, created_at, modifier, updated_at, label FROM mutation_snapshot_items WHERE id = ?`, id).
		Scan(&after, &creator, &createdAt, &modifier, &updatedAt, &label); err != nil {
		t.Fatalf("read ADD result: %v", err)
	}
	if creator != "integration-test" || modifier != "integration-test" || label != "created" {
		t.Fatalf("unexpected ADD values: creator=%q modifier=%q label=%q", creator, modifier, label)
	}
	if createdAt.Before(before) || createdAt.After(after) || !createdAt.Equal(updatedAt) {
		t.Fatalf("ADD did not use one database-time value for Create and Modify: before=%s created=%s updated=%s after=%s", before, createdAt, updatedAt, after)
	}

	clientManaged := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"client-managed","label":"bad","creator":"client"}}`)
	assertIntegrationErrorCode(t, clientManaged, http.StatusBadRequest, "invalid_mutation_content")
	assertDirectMutationCodeCount(t, ctx, database, "client-managed", 0)

	modified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_snapshot_items/rows/"+id, `{"content":{"label":"modified"}}`)
	assertMutationAffected(t, modified)
	var modifiedCreator, modifiedModifier, modifiedLabel string
	var modifiedCreatedAt, modifiedUpdatedAt time.Time
	if err := database.QueryRowContext(ctx, `SELECT creator, created_at, modifier, updated_at, label FROM mutation_snapshot_items WHERE id = ?`, id).
		Scan(&modifiedCreator, &modifiedCreatedAt, &modifiedModifier, &modifiedUpdatedAt, &modifiedLabel); err != nil {
		t.Fatalf("read MODIFY result: %v", err)
	}
	if modifiedCreator != creator || !modifiedCreatedAt.Equal(createdAt) || modifiedModifier != "integration-test" || modifiedLabel != "modified" || modifiedUpdatedAt.Before(updatedAt) {
		t.Fatalf("MODIFY changed Create fields or missed Modify fields: creator=%q created=%s modifier=%q updated=%s label=%q", modifiedCreator, modifiedCreatedAt, modifiedModifier, modifiedUpdatedAt, modifiedLabel)
	}
	clientModified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_snapshot_items/rows/"+id, `{"content":{"updated_at":"2000-01-01 00:00:00"}}`)
	assertIntegrationErrorCode(t, clientModified, http.StatusBadRequest, "invalid_mutation_content")

	deleted := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_snapshot_items/rows/"+id, "")
	assertMutationAffected(t, deleted)
	assertDirectMutationCodeCount(t, ctx, database, "full-lifecycle", 0)
}

func TestRelationalMutationPolicyIsSoleAuthorizationSourceAndDeprecatedExecutes(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/008-mutation-policy-snapshot-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	assignRelationalMutationPolicy(t, app, "snapshot_read_only_mutation_v1", false, false, false, false)

	denied := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"read-only","label":"must-deny"}}`)
	assertIntegrationErrorCode(t, denied, http.StatusForbidden, "mutation_not_allowed")

	replaceRelationalMutationPolicy(t, app, "snapshot_deprecated_mutation_v2", true, false, false, true)
	if _, err := app.mysql.SetMutationPolicyStatus(ctx, "snapshot_deprecated_mutation_v2", domain.PolicyStatusActive, domain.PolicyStatusDeprecated, "integration-test"); err != nil {
		t.Fatalf("deprecate assigned Mutation Policy: %v", err)
	}
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"deprecated","label":"still-executes"}}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("assigned Deprecated Mutation Policy did not execute: HTTP %d %s", added.Code, added.Body.String())
	}
}

func TestRelationalMutationPolicyFailsClosedAndRollsBackAtomically(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/008-mutation-policy-snapshot-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	assignRelationalMutationPolicy(t, app, "snapshot_rollback_mutation_v1", true, true, true, true)
	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open verification database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	failed := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"atomic-rollback","label":"rollback"}}`)
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	assertDirectMutationCodeCount(t, ctx, database, "atomic-rollback", 0)

	tests := []struct {
		name       string
		corrupt    string
		wantStatus int
		wantCode   string
	}{
		{name: "Draft reference", corrupt: `UPDATE rcc_mutation_policies SET status = 'DRAFT' WHERE code = 'snapshot_rollback_mutation_v1'`, wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_policy_snapshot"},
		{name: "unknown Type", corrupt: `UPDATE rcc_mutation_policies SET type_code = 'unknown_mutation' WHERE code = 'snapshot_rollback_mutation_v1'`, wantStatus: http.StatusUnprocessableEntity, wantCode: "unknown_policy_type"},
		{name: "missing definition", corrupt: `DELETE FROM rcc_mutation_policies WHERE code = 'snapshot_rollback_mutation_v1'`, wantStatus: http.StatusServiceUnavailable, wantCode: "policy_catalog_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := database.ExecContext(ctx, `UPDATE rcc_mutation_policies SET status = 'ACTIVE', type_code = 'single_table_mutation' WHERE code = 'snapshot_rollback_mutation_v1'`); err != nil {
				t.Fatalf("restore Mutation Policy: %v", err)
			}
			if _, err := database.ExecContext(ctx, test.corrupt); err != nil {
				t.Fatalf("corrupt Mutation Policy: %v", err)
			}
			response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"corrupt-`+test.name+`","label":"x"}}`)
			assertIntegrationErrorCode(t, response, test.wantStatus, test.wantCode)
		})
	}
}

func TestInFlightMutationKeepsOnePolicySnapshotWhileReplacementAffectsNextRequest(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/008-mutation-policy-snapshot-fixture.sql",
	)
	assignRelationalMutationPolicy(t, app, "snapshot_allowed_mutation_v1", true, false, false, true)
	createRelationalMutationDefinition(t, app, "snapshot_denied_mutation_v2", false, false, false, false)

	tableRead := make(chan struct{})
	resume := make(chan struct{})
	barrier := &mutationSnapshotBarrier{delegate: app.mysql, tableRead: tableRead, resumeTable: resume}
	installMutationSnapshotExecutor(t, app, barrier)

	responseChannel := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responseChannel <- policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"in-flight","label":"old-snapshot"}}`)
	}()
	waitForSnapshotBarrier(t, tableRead, "Mutation Table Policy read")
	replaced := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/mutation_snapshot_items", tablePolicyCodePayload("mutation_snapshot_items", "snapshot_mutation_query_v1", "snapshot_denied_mutation_v2"))
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace in-flight Mutation Policy: HTTP %d %s", replaced.Code, replaced.Body.String())
	}
	close(resume)
	select {
	case response := <-responseChannel:
		if response.Code != http.StatusCreated {
			t.Fatalf("in-flight mutation lost its original Snapshot: HTTP %d %s", response.Code, response.Body.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("in-flight mutation did not finish")
	}
	next := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"next","label":"new-snapshot"}}`)
	assertIntegrationErrorCode(t, next, http.StatusForbidden, "mutation_not_allowed")
}

func TestMutationSnapshotSessionCannotBeReusedAfterTransaction(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/008-mutation-policy-snapshot-fixture.sql",
	)
	var captured application.MutationSnapshotSession
	err := app.mysql.ExecuteMutationSnapshot(t.Context(), func(session application.MutationSnapshotSession) error {
		captured = session
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected callback error to roll back Mutation Snapshot")
	}
	if _, err := captured.GetTablePolicy(t.Context(), "mutation_snapshot_items"); !errors.Is(err, application.ErrMutationUnavailable) {
		t.Fatalf("Mutation Snapshot session remained reusable: %v", err)
	}
}

func assignRelationalMutationPolicy(t *testing.T, app *adminApplication, mutationCode string, allowAdd, allowModify, allowDelete, autoFill bool) {
	t.Helper()
	queryPolicy := relationalQueryPolicyFixture("snapshot_mutation_query_v1", queryPolicyFixture{})
	if _, err := app.mysql.CreateQueryPolicy(t.Context(), queryPolicy, "integration-test"); err != nil && !errors.Is(err, domain.ErrQueryPolicyExists) {
		t.Fatalf("create relational Query Policy: %v", err)
	}
	if _, err := app.mysql.SetQueryPolicyStatus(t.Context(), queryPolicy.Code, domain.PolicyStatusDraft, domain.PolicyStatusActive, "integration-test"); err != nil && !errors.Is(err, domain.ErrQueryPolicyStateConflict) {
		t.Fatalf("activate relational Query Policy: %v", err)
	}
	createRelationalMutationDefinition(t, app, mutationCode, allowAdd, allowModify, allowDelete, autoFill)
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", tablePolicyCodePayload("mutation_snapshot_items", queryPolicy.Code, mutationCode))
	if created.Code != http.StatusCreated {
		t.Fatalf("create relational Table Policy: HTTP %d %s", created.Code, created.Body.String())
	}
	enabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/mutation_snapshot_items/enable", "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable relational Table Policy: HTTP %d %s", enabled.Code, enabled.Body.String())
	}
}

func replaceRelationalMutationPolicy(t *testing.T, app *adminApplication, code string, allowAdd, allowModify, allowDelete, autoFill bool) {
	t.Helper()
	createRelationalMutationDefinition(t, app, code, allowAdd, allowModify, allowDelete, autoFill)
	response := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/mutation_snapshot_items", tablePolicyCodePayload("mutation_snapshot_items", "snapshot_mutation_query_v1", code))
	if response.Code != http.StatusOK {
		t.Fatalf("replace relational Mutation Policy: HTTP %d %s", response.Code, response.Body.String())
	}
}

func createRelationalMutationDefinition(t *testing.T, app *adminApplication, code string, allowAdd, allowModify, allowDelete, autoFill bool) {
	t.Helper()
	policy := domain.MutationPolicy{
		Code: code, Name: "Snapshot Mutation", Description: "Issue 19 integration", TypeCode: application.SingleTableMutationPolicyType,
		AllowAdd: allowAdd, AllowModify: allowModify, AllowDelete: allowDelete,
	}
	if autoFill {
		createOperator, createTime, modifyOperator, modifyTime := "creator", "created_at", "modifier", "updated_at"
		policy.CreateOperatorField, policy.CreateTimeField = &createOperator, &createTime
		policy.ModifyOperatorField, policy.ModifyTimeField = &modifyOperator, &modifyTime
	}
	if _, err := app.mysql.CreateMutationPolicy(t.Context(), policy, "integration-test"); err != nil {
		t.Fatalf("create relational Mutation Policy %s: %v", code, err)
	}
	if _, err := app.mysql.SetMutationPolicyStatus(t.Context(), code, domain.PolicyStatusDraft, domain.PolicyStatusActive, "integration-test"); err != nil {
		t.Fatalf("activate relational Mutation Policy %s: %v", code, err)
	}
}

func assertDirectMutationCodeCount(t *testing.T, ctx context.Context, database *sql.DB, code string, expected int) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM mutation_snapshot_items WHERE code = ?", code).Scan(&count); err != nil {
		t.Fatalf("count mutation snapshot row: %v", err)
	}
	if count != expected {
		t.Fatalf("expected code %q count %d, got %d", code, expected, count)
	}
}

type mutationSnapshotBarrier struct {
	delegate    application.MutationSnapshotExecutor
	tableRead   chan struct{}
	resumeTable <-chan struct{}
	tableOnce   sync.Once
}

func (barrier *mutationSnapshotBarrier) ExecuteMutationSnapshot(ctx context.Context, execute func(application.MutationSnapshotSession) error) error {
	return barrier.delegate.ExecuteMutationSnapshot(ctx, func(session application.MutationSnapshotSession) error {
		return execute(&mutationSnapshotBarrierSession{MutationSnapshotSession: session, barrier: barrier})
	})
}

type mutationSnapshotBarrierSession struct {
	application.MutationSnapshotSession
	barrier *mutationSnapshotBarrier
}

func (session *mutationSnapshotBarrierSession) GetTablePolicy(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	policy, err := session.MutationSnapshotSession.GetTablePolicy(ctx, tableName)
	if err == nil {
		session.barrier.tableOnce.Do(func() {
			close(session.barrier.tableRead)
			select {
			case <-session.barrier.resumeTable:
			case <-ctx.Done():
			}
		})
	}
	return policy, err
}

func installMutationSnapshotExecutor(t *testing.T, app *adminApplication, executor application.MutationSnapshotExecutor) {
	t.Helper()
	discovery := application.NewDatabaseTableDiscovery(app.mysql)
	queryPolicies := application.NewQueryPolicyManagement(app.mysql, application.NewQueryPolicyTypeRegistry(), "integration-test")
	mutationPolicies := application.NewMutationPolicyManagement(app.mysql, application.NewMutationPolicyTypeRegistry(), "integration-test")
	policies := application.NewTablePolicyManagement(app.mysql, app.mysql, queryPolicies, mutationPolicies, "integration-test")
	queries := application.NewManagedTableQuery(app.mysql, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry())
	mutations := application.NewManagedTableMutation(executor, application.NewQueryPolicyTypeRegistry(), application.NewMutationPolicyTypeRegistry(), application.NewFixedOperatorProvider("integration-test"))
	app.handler = httpinterface.NewRouter(discovery, app.mysql, queryPolicies, mutationPolicies, policies, queries, mutations, httpinterface.RouterOptions{AuthDisabled: true, AccessLog: io.Discard})
}

var _ application.MutationSnapshotExecutor = (*mutationSnapshotBarrier)(nil)

func TestMutationPolicyAddsRowAndReturnsJSONStringID(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})

	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"first","label":"created through Admin"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row: expected HTTP 201, got %d: %s", added.Code, added.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if created.ID != "1" {
		t.Fatalf("expected lossless string id 1, got %q", created.ID)
	}

	queried := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/query", `{
		"conditions":[{"field":"id","operator":"exact","value":"1"}]
	}`)
	if queried.Code != http.StatusOK {
		t.Fatalf("query inserted row: expected HTTP 200, got %d: %s", queried.Code, queried.Body.String())
	}
	var result struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(queried.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["code"] == nil || *result.Rows[0]["code"] != "first" || result.Rows[0]["label"] == nil || *result.Rows[0]["label"] != "created through Admin" {
		t.Fatalf("inserted row was not observable through Admin query: %s", queried.Body.String())
	}
}

func TestMutationPolicyReturnsExplicitAutoIncrementID(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})

	const explicitID = "9007199254740993"
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"id":"`+explicitID+`","code":"explicit-auto-id","label":"client supplied id"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row with explicit auto-increment id: HTTP %d %s", added.Code, added.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if response.ID != explicitID {
		t.Fatalf("expected explicit lossless id %q, got %q", explicitID, response.ID)
	}

	row := queryMutationRow(t, app, "explicit-auto-id")
	assertMutationString(t, row, "id", explicitID)
}

func TestMutationPolicyReturnsARequiredNonAutoIncrementID(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true})

	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_supplied_id_items/rows", `{
		"content":{"id":"external-9007199254740993","label":"supplied identity"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row with supplied id: HTTP %d %s", added.Code, added.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if response.ID != "external-9007199254740993" {
		t.Fatalf("expected supplied lossless id, got %q", response.ID)
	}
}

func TestMutationPolicyUsesDefaultsAndNullabilityAndRejectsInvalidInputFields(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})

	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"schema-rules","label":"valid","nullable_value":null,"quantity":"7","metadata":"{\"enabled\":true}"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row using default and NULL semantics: HTTP %d %s", added.Code, added.Body.String())
	}

	queried := queryMutationRow(t, app, "schema-rules")
	if queried["defaulted_value"] == nil || *queried["defaulted_value"] != "database-default" {
		t.Fatalf("database default was not preserved: %#v", queried)
	}
	if queried["nullable_value"] != nil {
		t.Fatalf("explicit JSON null did not become SQL NULL: %#v", queried)
	}
	if queried["generated_value"] == nil || *queried["generated_value"] != "schema-rules:generated" {
		t.Fatalf("generated value was not produced by MySQL: %#v", queried)
	}

	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "unknown field", body: `{"content":{"code":"unknown","label":"bad","not_a_column":"x"}}`, code: "invalid_mutation_content"},
		{name: "generated field", body: `{"content":{"code":"generated","label":"bad","generated_value":"override"}}`, code: "invalid_mutation_content"},
		{name: "invalid integer", body: `{"content":{"code":"integer","label":"bad","quantity":"seven"}}`, code: "invalid_mutation_content"},
		{name: "invalid enum member", body: `{"content":{"code":"enum","label":"bad","lifecycle":"retired"}}`, code: "invalid_mutation_content"},
		{name: "invalid JSON", body: `{"content":{"code":"json","label":"bad","metadata":"not-json"}}`, code: "invalid_mutation_content"},
		{name: "null in non-null column", body: `{"content":{"code":"null","label":null}}`, code: "invalid_mutation_content"},
		{name: "non-string dynamic value", body: `{"content":{"code":"number","label":7}}`, code: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", test.body)
			assertIntegrationErrorCode(t, response, http.StatusBadRequest, test.code)
		})
	}
}

func TestMutationPolicyAutoFillUsesFixedOperatorAndDatabaseTimeAndRejectsManagedInput(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{
		AllowAdd: true, CreateOperatorField: stringPointer("creator"), CreateTimeField: stringPointer("occurred_at"),
	})

	before := time.Now().UTC().Add(-time.Second)
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_auto_fill_items/rows", `{
		"content":{
			"code":"auto-fill",
			"status":"client",
			"quantity":"1"
		}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row with Auto Fill: HTTP %d %s", added.Code, added.Body.String())
	}
	after := time.Now().UTC().Add(time.Second)

	row := queryMutationTableRow(t, app, "mutation_auto_fill_items", "auto-fill")
	assertMutationString(t, row, "creator", "integration-test")
	assertMutationString(t, row, "status", "client")
	assertMutationString(t, row, "quantity", "1")
	if row["occurred_at"] == nil {
		t.Fatalf("Auto Fill now produced SQL NULL: %#v", row)
	}
	occurredAt, err := time.ParseInLocation("2006-01-02 15:04:05.999999", *row["occurred_at"], time.UTC)
	if err != nil || occurredAt.Before(before) || occurredAt.After(after) {
		t.Fatalf("Auto Fill now value %q is outside request window [%s, %s]: %v", *row["occurred_at"], before, after, err)
	}
	rejected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_auto_fill_items/rows", `{
		"content":{"code":"managed-input","creator":"client","status":"client","quantity":"1"}
	}`)
	assertIntegrationErrorCode(t, rejected, http.StatusBadRequest, "invalid_mutation_content")
}

func TestMutationPolicyRejectsMissingRequiredFieldsAndRollsBackDatabaseFailures(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})

	missing := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"missing-label"}
	}`)
	assertIntegrationErrorCode(t, missing, http.StatusBadRequest, "missing_required_field")
	assertMutationRowAbsent(t, app, "mutation_add_items", "missing-label")

	failed := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"rollback","label":"rollback"}
	}`)
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	assertMutationRowAbsent(t, app, "mutation_add_items", "rollback")
}

func TestMutationPolicyMapsMySQLUniqueKeyViolationsToConflict(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})

	first := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"duplicate","label":"first"}
	}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("create first unique row: HTTP %d %s", first.Code, first.Body.String())
	}
	duplicate := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"duplicate","label":"second"}
	}`)
	assertIntegrationErrorCode(t, duplicate, http.StatusConflict, "duplicate_key")
	row := queryMutationRow(t, app, "duplicate")
	assertMutationString(t, row, "label", "first")
}

func TestMutationPolicyPatchesOnlySubmittedFieldsWithJSONStringSemantics(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	id := addMutationPatchFixtureRow(t, app, "patch-semantics", "original")

	modified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"label":"changed"}
	}`)
	assertMutationAffected(t, modified)
	row := queryMutationRow(t, app, "patch-semantics")
	assertMutationString(t, row, "label", "changed")
	assertMutationString(t, row, "nullable_value", "preserved")
	assertMutationString(t, row, "quantity", "7")
	assertMutationString(t, row, "defaulted_value", "database-default")

	nulled := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"nullable_value":null}
	}`)
	assertMutationAffected(t, nulled)
	if row = queryMutationRow(t, app, "patch-semantics"); row["nullable_value"] != nil {
		t.Fatalf("PATCH JSON null did not become SQL NULL: %#v", row)
	}

	emptied := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"label":""}
	}`)
	assertMutationAffected(t, emptied)
	row = queryMutationRow(t, app, "patch-semantics")
	assertMutationString(t, row, "label", "")

	unchanged := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"label":""}
	}`)
	assertMutationAffected(t, unchanged)
}

func TestMutationPolicyPatchRejectsInvalidAndNonWritableFields(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	id := addMutationPatchFixtureRow(t, app, "patch-invalid", "original")

	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "invalid integer", body: `{"content":{"quantity":"seven"}}`, code: "invalid_mutation_content"},
		{name: "primary key", body: `{"content":{"id":"99"}}`, code: "invalid_mutation_content"},
		{name: "generated field", body: `{"content":{"generated_value":"override"}}`, code: "invalid_mutation_content"},
		{name: "unknown field", body: `{"content":{"not_a_column":"value"}}`, code: "invalid_mutation_content"},
		{name: "null in non-null column", body: `{"content":{"label":null}}`, code: "invalid_mutation_content"},
		{name: "empty patch", body: `{"content":{}}`, code: "invalid_mutation_content"},
		{name: "non-string dynamic value", body: `{"content":{"quantity":7}}`, code: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, test.body)
			assertIntegrationErrorCode(t, response, http.StatusBadRequest, test.code)
			row := queryMutationRow(t, app, "patch-invalid")
			assertMutationString(t, row, "label", "original")
			assertMutationString(t, row, "quantity", "7")
		})
	}

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` ADD COLUMN `unsupported_value` blob NULL"); err != nil {
		t.Fatalf("add unsupported live field: %v", err)
	}
	unsupported := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"unsupported_value":"bytes"}}`)
	assertIntegrationErrorCode(t, unsupported, http.StatusUnprocessableEntity, "incompatible_table")
}

func TestMutationPolicyPatchUsesLatestPolicyAndLiveSchema(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	id := addMutationPatchFixtureRow(t, app, "patch-latest", "original")

	forbidden := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"forbidden"}}`)
	assertIntegrationErrorCode(t, forbidden, http.StatusForbidden, "mutation_not_allowed")

	replacePolicyAssignment(t, app, "mutation_add_items", queryPolicyFixture{}, mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` ADD COLUMN `future_note` varchar(64) NULL DEFAULT 'future-default'"); err != nil {
		t.Fatalf("add supported live field: %v", err)
	}

	modified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"latest"}}`)
	assertMutationAffected(t, modified)
	row := queryMutationRow(t, app, "patch-latest")
	assertMutationString(t, row, "label", "latest")
	assertMutationString(t, row, "future_note", "future-default")
	assertMutationString(t, row, "defaulted_value", "database-default")

	newField := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"future_note":"live-schema"}}`)
	assertMutationAffected(t, newField)
	row = queryMutationRow(t, app, "patch-latest")
	assertMutationString(t, row, "future_note", "live-schema")
	assertMutationString(t, row, "defaulted_value", "database-default")

	missing := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/999999", `{"content":{"label":"missing"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "mutation_row_not_found")
}

func TestMutationPolicyPatchAutoFillUsesModifySlotsAndRejectsManagedInput(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{
		AllowAdd: true, AllowModify: true,
		ModifyOperatorField: stringPointer("creator"), ModifyTimeField: stringPointer("occurred_at"),
	})
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_auto_fill_items/rows", `{
		"content":{"code":"patch-auto-fill","status":"initial","quantity":"1"}
	}`)
	id := mutationResponseID(t, added)
	created := queryMutationTableRow(t, app, "mutation_auto_fill_items", "patch-auto-fill")
	assertMutationString(t, created, "creator", "integration-test")
	if created["occurred_at"] == nil {
		t.Fatalf("ADD did not apply configured Modify Auto Fill slots: %#v", created)
	}

	before := time.Now().UTC().Add(-time.Second)
	modified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_auto_fill_items/rows/"+id, `{
		"content":{"status":"client","quantity":"2"}
	}`)
	assertMutationAffected(t, modified)
	after := time.Now().UTC().Add(time.Second)

	row := queryMutationTableRow(t, app, "mutation_auto_fill_items", "patch-auto-fill")
	assertMutationString(t, row, "creator", "integration-test")
	assertMutationString(t, row, "status", "client")
	assertMutationString(t, row, "quantity", "2")
	if row["occurred_at"] == nil {
		t.Fatalf("MODIFY Auto Fill now produced SQL NULL: %#v", row)
	}
	occurredAt, err := time.ParseInLocation("2006-01-02 15:04:05.999999", *row["occurred_at"], time.UTC)
	if err != nil || occurredAt.Before(before) || occurredAt.After(after) {
		t.Fatalf("MODIFY Auto Fill now value %q is outside request window [%s, %s]: %v", *row["occurred_at"], before, after, err)
	}
	rejected := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_auto_fill_items/rows/"+id, `{"content":{"creator":"client"}}`)
	assertIntegrationErrorCode(t, rejected, http.StatusBadRequest, "invalid_mutation_content")
}

func TestMutationPolicyPatchRollsBackDatabaseFailures(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	id := addMutationPatchFixtureRow(t, app, "patch-rollback", "original")

	failed := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"rollback","quantity":"99"}}`)
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	row := queryMutationRow(t, app, "patch-rollback")
	assertMutationString(t, row, "label", "original")
	assertMutationString(t, row, "quantity", "7")
}

func TestMutationPolicyPatchMapsUniqueKeyConflictsAndRollsBack(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	addMutationPatchFixtureRow(t, app, "patch-unique-first", "first")
	secondID := addMutationPatchFixtureRow(t, app, "patch-unique-second", "second")

	conflict := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+secondID, `{"content":{"code":"patch-unique-first","label":"changed"}}`)
	assertIntegrationErrorCode(t, conflict, http.StatusConflict, "duplicate_key")
	row := queryMutationRow(t, app, "patch-unique-second")
	assertMutationString(t, row, "label", "second")
}

func TestMutationPolicyPatchFailsClosedForCurrentPolicyAndSchema(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	missing := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/1", `{"content":{"label":"missing"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "table_policy_not_found")
	createPolicyAssignment(t, app, "mutation_add_items", queryPolicyFixture{}, mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	disabled := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/1", `{"content":{"label":"disabled"}}`)
	assertIntegrationErrorCode(t, disabled, http.StatusForbidden, "table_policy_disabled")
	setPolicyAssignmentEnabled(t, app, "mutation_add_items", true)
	id := addMutationPatchFixtureRow(t, app, "patch-fail-closed", "original")

	protected := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/rcc_table_policies/rows/1", `{"content":{"modifier":"x"}}`)
	assertIntegrationErrorCode(t, protected, http.StatusForbidden, "protected_table")

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	assignment, err := app.mysql.Get(ctx, "mutation_add_items")
	if err != nil {
		t.Fatalf("read current Table Policy: %v", err)
	}
	if _, err := database.ExecContext(ctx, "UPDATE `rcc_mutation_policies` SET `type_code` = 'unknown_mutation' WHERE `code` = ?", assignment.MutationPolicyCode); err != nil {
		t.Fatalf("make current Mutation Type invalid: %v", err)
	}
	unknown := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"unknown"}}`)
	assertIntegrationErrorCode(t, unknown, http.StatusUnprocessableEntity, "unknown_policy_type")

	if _, err := database.ExecContext(ctx, "UPDATE `rcc_mutation_policies` SET `type_code` = 'single_table_mutation' WHERE `code` = ?", assignment.MutationPolicyCode); err != nil {
		t.Fatalf("restore current Mutation Type: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` MODIFY `id` bigint unsigned NOT NULL, DROP PRIMARY KEY"); err != nil {
		t.Fatalf("make live Schema incompatible: %v", err)
	}
	incompatible := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"incompatible"}}`)
	assertIntegrationErrorCode(t, incompatible, http.StatusUnprocessableEntity, "incompatible_table")
}

func TestMutationPolicyDeleteDefaultsToDeniedThenUsesTheCurrentReplacement(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	id := addMutationPatchFixtureRow(t, app, "delete-current", "preserved until permitted")

	denied := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertIntegrationErrorCode(t, denied, http.StatusForbidden, "mutation_not_allowed")
	assertMutationString(t, queryMutationRow(t, app, "delete-current"), "label", "preserved until permitted")

	replacePolicyAssignment(t, app, "mutation_add_items", queryPolicyFixture{}, mutationPolicyFixture{AllowAdd: true, AllowDelete: true})

	deleted := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertMutationAffected(t, deleted)
	assertMutationRowAbsent(t, app, "mutation_add_items", "delete-current")
}

func TestMutationPolicyDeleteFailsClosedBeforeExecution(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)

	missingPolicy := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/1", "")
	assertIntegrationErrorCode(t, missingPolicy, http.StatusNotFound, "table_policy_not_found")

	createPolicyAssignment(t, app, "mutation_add_items", queryPolicyFixture{}, mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	disabled := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/1", "")
	assertIntegrationErrorCode(t, disabled, http.StatusForbidden, "table_policy_disabled")
	setPolicyAssignmentEnabled(t, app, "mutation_add_items", true)
	id := addMutationPatchFixtureRow(t, app, "delete-invalid-id", "must remain")

	invalidID := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/not-an-integer", "")
	assertIntegrationErrorCode(t, invalidID, http.StatusBadRequest, "invalid_mutation_content")
	assertMutationString(t, queryMutationRow(t, app, "delete-invalid-id"), "id", id)

	protected := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/rcc_table_policies/rows/1", "")
	assertIntegrationErrorCode(t, protected, http.StatusForbidden, "protected_table")
}

func TestMutationPolicyDeleteMapsMissingRowsToNotFound(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowDelete: true})

	missing := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/999999", "")
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "mutation_row_not_found")
}

func TestMutationPolicyDeleteRollsBackDatabaseFailures(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowDelete: true})

	failed := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_delete_parents/rows/1", "")
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	row := queryMutationTableRow(t, app, "mutation_delete_parents", "delete-rollback")
	assertMutationString(t, row, "id", "1")
}

func TestMutationPolicyDeleteFailsClosedForUnsupportedLiveColumns(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	id := addMutationPatchFixtureRow(t, app, "delete-blob", "delete despite blob")

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` ADD COLUMN `unsupported_payload` BLOB NULL"); err != nil {
		t.Fatalf("add unsupported unrelated column: %v", err)
	}

	queryRejected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/query", `{}`)
	assertIntegrationErrorCode(t, queryRejected, http.StatusUnprocessableEntity, "incompatible_table")
	deleted := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertIntegrationErrorCode(t, deleted, http.StatusUnprocessableEntity, "incompatible_table")

	var count int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM `mutation_add_items` WHERE `id` = ?", id).Scan(&count); err != nil {
		t.Fatalf("verify deleted row directly: %v", err)
	}
	if count != 1 {
		t.Fatalf("row changed despite incompatible complete Policy Snapshot: count=%d", count)
	}
}

func TestMutationPolicyDeleteFailsClosedForStaleAutoFillSchemaRules(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{
		AllowModify: true, AllowDelete: true, ModifyOperatorField: stringPointer("creator"),
	})

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, `
INSERT INTO mutation_auto_fill_items (id, code, creator, occurred_at, status, quantity)
VALUES (1, 'delete-stale-autofill', 'initial', '2026-01-01 00:00:00', 'initial', 1)`); err != nil {
		t.Fatalf("seed Auto Fill delete row: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_auto_fill_items` DROP COLUMN `creator`"); err != nil {
		t.Fatalf("remove now-unrelated Auto Fill column: %v", err)
	}

	deleted := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_auto_fill_items/rows/1", "")
	assertIntegrationErrorCode(t, deleted, http.StatusUnprocessableEntity, "invalid_policy_snapshot")
	assertDirectMutationRowCount(t, ctx, database, "mutation_auto_fill_items", "1", 1)
}

func TestMutationPolicyDeleteFailsClosedForInvalidCurrentPolicyAndLiveTable(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	id := addMutationPatchFixtureRow(t, app, "delete-invalid-policy", "must remain")
	assignment, err := app.mysql.Get(ctx, "mutation_add_items")
	if err != nil {
		t.Fatalf("read current Table Policy: %v", err)
	}
	if _, err := database.ExecContext(ctx, "UPDATE `rcc_mutation_policies` SET `type_code` = 'unknown_mutation' WHERE `code` = ?", assignment.MutationPolicyCode); err != nil {
		t.Fatalf("make current Mutation Type unknown: %v", err)
	}
	unknown := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertIntegrationErrorCode(t, unknown, http.StatusUnprocessableEntity, "unknown_policy_type")
	assertDirectMutationRowCount(t, ctx, database, "mutation_add_items", id, 1)

	if _, err := database.ExecContext(ctx, "UPDATE `rcc_mutation_policies` SET `type_code` = 'single_table_mutation', `modify_operator_field` = 'missing_column' WHERE `code` = ?", assignment.MutationPolicyCode); err != nil {
		t.Fatalf("make current Auto Fill target invalid: %v", err)
	}
	malformed := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertIntegrationErrorCode(t, malformed, http.StatusUnprocessableEntity, "invalid_policy_snapshot")
	assertDirectMutationRowCount(t, ctx, database, "mutation_add_items", id, 1)

	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowDelete: true})
	if _, err := database.ExecContext(ctx, "INSERT INTO `mutation_supplied_id_items` (`id`, `label`) VALUES ('incompatible-id', 'must remain')"); err != nil {
		t.Fatalf("seed incompatible Schema row: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_supplied_id_items` DROP PRIMARY KEY"); err != nil {
		t.Fatalf("make live Schema incompatible: %v", err)
	}
	incompatible := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_supplied_id_items/rows/incompatible-id", "")
	assertIntegrationErrorCode(t, incompatible, http.StatusUnprocessableEntity, "incompatible_table")
	assertDirectMutationRowCount(t, ctx, database, "mutation_supplied_id_items", "incompatible-id", 1)

	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{AllowDelete: true})
	if _, err := database.ExecContext(ctx, "DROP TABLE `mutation_auto_fill_items`"); err != nil {
		t.Fatalf("drop current physical table: %v", err)
	}
	missingTable := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_auto_fill_items/rows/1", "")
	assertIntegrationErrorCode(t, missingTable, http.StatusNotFound, "database_table_not_found")
}

func TestMutationAddFailsClosedAndUsesTheLatestPolicySnapshot(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)

	missing := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"missing","label":"x"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "table_policy_not_found")

	createPolicyAssignment(t, app, "mutation_add_items", queryPolicyFixture{}, mutationPolicyFixture{})
	disabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"disabled","label":"x"}}`)
	assertIntegrationErrorCode(t, disabled, http.StatusForbidden, "table_policy_disabled")
	setPolicyAssignmentEnabled(t, app, "mutation_add_items", true)
	forbidden := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"forbidden","label":"x"}}`)
	assertIntegrationErrorCode(t, forbidden, http.StatusForbidden, "mutation_not_allowed")

	replacePolicyAssignment(t, app, "mutation_add_items", queryPolicyFixture{}, mutationPolicyFixture{AllowAdd: true})
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"current","label":"latest Policy"}}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("latest Policy Snapshot did not permit ADD: HTTP %d %s", added.Code, added.Body.String())
	}

	protected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/rcc_table_policies/rows", `{"content":{}}`)
	assertIntegrationErrorCode(t, protected, http.StatusForbidden, "protected_table")
}

func TestMutationAddFailsClosedForInvalidLivePolicyAndSchema(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	assignment, err := app.mysql.Get(ctx, "mutation_add_items")
	if err != nil {
		t.Fatalf("read current Table Policy: %v", err)
	}
	if _, err := database.ExecContext(ctx, "UPDATE `rcc_mutation_policies` SET `type_code` = 'unknown_mutation' WHERE `code` = ?", assignment.MutationPolicyCode); err != nil {
		t.Fatalf("make current Mutation Type invalid: %v", err)
	}
	unknown := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"unknown-policy","label":"x"}}`)
	assertIntegrationErrorCode(t, unknown, http.StatusUnprocessableEntity, "unknown_policy_type")

	if _, err := database.ExecContext(ctx, "UPDATE `rcc_mutation_policies` SET `type_code` = 'single_table_mutation' WHERE `code` = ?", assignment.MutationPolicyCode); err != nil {
		t.Fatalf("restore current Mutation Type: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` MODIFY `id` bigint unsigned NOT NULL, DROP PRIMARY KEY"); err != nil {
		t.Fatalf("make live Schema incompatible: %v", err)
	}
	incompatible := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"incompatible","label":"x"}}`)
	assertIntegrationErrorCode(t, incompatible, http.StatusUnprocessableEntity, "incompatible_table")
}

func TestMutationAddFailsClosedWhenPolicyCatalogIsUnavailable(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(context.Background(), "DROP TABLE `rcc_table_policies`"); err != nil {
		t.Fatalf("make Policy Catalog unavailable: %v", err)
	}

	rejected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"unavailable","label":"x"}}`)
	assertIntegrationErrorCode(t, rejected, http.StatusServiceUnavailable, "policy_catalog_unavailable")
}

func queryMutationRow(t *testing.T, app *adminApplication, code string) map[string]*string {
	return queryMutationTableRow(t, app, "mutation_add_items", code)
}

func addMutationPatchFixtureRow(t *testing.T, app *adminApplication, code, label string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"content": map[string]any{
			"code":           code,
			"label":          label,
			"nullable_value": "preserved",
			"quantity":       "7",
		},
	})
	if err != nil {
		t.Fatalf("encode mutation fixture row: %v", err)
	}
	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", string(body))
	return mutationResponseID(t, response)
}

func mutationResponseID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusCreated {
		t.Fatalf("add mutation fixture row: HTTP %d %s", response.Code, response.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode mutation fixture id: %v", err)
	}
	if body.ID == "" {
		t.Fatalf("mutation fixture returned empty id: %s", response.Body.String())
	}
	return body.ID
}

func assertMutationAffected(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("modify row: expected HTTP 200, got %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Affected int64 `json:"affected"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode modify response: %v", err)
	}
	if body.Affected != 1 {
		t.Fatalf("modify row: expected affected 1, got %d: %s", body.Affected, response.Body.String())
	}
}

func queryMutationTableRow(t *testing.T, app *adminApplication, tableName, code string) map[string]*string {
	t.Helper()
	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/"+tableName+"/query", `{
		"conditions":[{"field":"code","operator":"exact","value":"`+code+`"}]
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("query mutation row %q: HTTP %d %s", code, response.Code, response.Body.String())
	}
	var result struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected one row for %q, got %s", code, response.Body.String())
	}
	return result.Rows[0]
}

func assertMutationString(t *testing.T, row map[string]*string, field, expected string) {
	t.Helper()
	if row[field] == nil || *row[field] != expected {
		t.Fatalf("expected %s=%q, got %#v", field, expected, row[field])
	}
}

func assertMutationRowAbsent(t *testing.T, app *adminApplication, tableName, code string) {
	t.Helper()
	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/"+tableName+"/query", `{
		"conditions":[{"field":"code","operator":"exact","value":"`+code+`"}]
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("query absent mutation row %q: HTTP %d %s", code, response.Code, response.Body.String())
	}
	var result struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("mutation row %q should be absent after rejection/rollback: %s", code, response.Body.String())
	}
}

func assertDirectMutationRowCount(t *testing.T, ctx context.Context, database *sql.DB, tableName, id string, expected int) {
	t.Helper()
	var count int
	query := "SELECT COUNT(*) FROM `" + tableName + "` WHERE `id` = ?"
	if err := database.QueryRowContext(ctx, query, id).Scan(&count); err != nil {
		t.Fatalf("count direct mutation row: %v", err)
	}
	if count != expected {
		t.Fatalf("expected direct row count %d, got %d", expected, count)
	}
}

func enableMutationPolicy(t *testing.T, app *adminApplication, tableName string, mutation mutationPolicyFixture) {
	t.Helper()
	enablePolicyAssignment(t, app, tableName, queryPolicyFixture{}, mutation)
}
