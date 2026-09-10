//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

const localManagedTableFixture = "../../../deploy/mysql/local-fixture/002-notification-templates.sql"

func TestLocalManagedTableFixtureIsIdempotentAndImmediatelyUsable(t *testing.T) {
	ctx, driverConfig := startCurrentIntegrationMySQL(t, localManagedTableFixture)
	applyLocalManagedTableFixture(t, driverConfig)

	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin with local Managed Table fixture: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	discovery := policyIntegrationRequest(t, app, http.MethodGet, "/api/v1/database-tables", "")
	if discovery.Code != http.StatusOK {
		t.Fatalf("discover local Managed Table: HTTP %d %s", discovery.Code, discovery.Body.String())
	}
	var discovered struct {
		Tables []struct {
			TableName     string `json:"table_name"`
			PolicyExists  bool   `json:"policy_exists"`
			PolicyEnabled bool   `json:"policy_enabled"`
			Compatible    bool   `json:"compatible"`
		} `json:"tables"`
	}
	decodeFixtureResponse(t, discovery.Body.Bytes(), &discovered)
	foundNotificationTemplates := false
	for _, table := range discovered.Tables {
		if strings.HasPrefix(table.TableName, "rcc_") {
			t.Fatalf("protected Policy Catalog table leaked through Discovery: %q", table.TableName)
		}
		if table.TableName == "notification_templates" {
			foundNotificationTemplates = true
			if !table.PolicyExists || !table.PolicyEnabled || !table.Compatible {
				t.Fatalf("local table is not an enabled compatible Managed Table: %#v", table)
			}
		}
	}
	if !foundNotificationTemplates {
		t.Fatalf("notification_templates missing from Discovery: %s", discovery.Body.String())
	}

	assertLocalFixturePolicies(t, app)

	page := policyIntegrationRequest(t, app, http.MethodPost, "/api/v1/tables/notification_templates/query", `{
		"order":{"field":"id","direction":"ASC"},
		"page_number":2,
		"page_size":2
	}`)
	if page.Code != http.StatusOK {
		t.Fatalf("query local Managed Table: HTTP %d %s", page.Code, page.Body.String())
	}
	var queryResult struct {
		Rows []map[string]*string `json:"rows"`
		Page struct {
			PageNumber int   `json:"page_number"`
			PageSize   int   `json:"page_size"`
			TotalCount int64 `json:"total_count"`
			TotalPages int64 `json:"total_pages"`
		} `json:"page"`
	}
	decodeFixtureResponse(t, page.Body.Bytes(), &queryResult)
	if queryResult.Page.PageNumber != 2 || queryResult.Page.PageSize != 2 || queryResult.Page.TotalCount != 3 || queryResult.Page.TotalPages != 2 || len(queryResult.Rows) != 1 {
		t.Fatalf("fixture was duplicated or pagination is unusable: %s", page.Body.String())
	}

	added := publicationFixtureRequest(t, app, "ADD", "notification_templates", "", `{
		"content":{
			"template_key":"issue-25-cleanup",
			"channel":"EMAIL",
			"subject":"Fixture verification",
			"body":"created",
			"enabled":"1",
			"priority":"99",
			"metadata":"{\"source\":\"issue-25\"}"
		}
	}`)
	id := mutationResponseID(t, added)
	created := queryNotificationTemplate(t, app, "issue-25-cleanup")
	assertMutationString(t, created, "body", "created")
	assertMutationString(t, created, "creator", integrationAccountID(t, app))
	assertMutationString(t, created, "modifier", integrationAccountID(t, app))

	modified := versionedPublicationFixture(t, app, "MODIFY", "notification_templates", id, `{"content":{"body":"modified"}}`)
	assertMutationAffected(t, modified)
	assertMutationString(t, queryNotificationTemplate(t, app, "issue-25-cleanup"), "body", "modified")

	deleted := versionedPublicationFixture(t, app, "DELETE", "notification_templates", id, "")
	assertMutationAffected(t, deleted)
	assertNotificationTemplateAbsent(t, app, "issue-25-cleanup")
}

func TestLocalManagedTableFixtureRejectsLifecycleConflictsWithoutReactivation(t *testing.T) {
	ctx, driverConfig := startCurrentIntegrationMySQL(t, localManagedTableFixture)
	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture verification database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, `UPDATE rcc_query_policies SET status = 'DEPRECATED' WHERE code = 'notification_page_query_v1'`); err != nil {
		t.Fatalf("deprecate local Query Policy: %v", err)
	}

	if err := executeLocalManagedTableFixture(driverConfig); err == nil {
		t.Fatal("expected conflicting existing Policy lifecycle to reject fixture reapplication")
	}
	var status string
	if err := database.QueryRowContext(ctx, `SELECT status FROM rcc_query_policies WHERE code = 'notification_page_query_v1'`).Scan(&status); err != nil {
		t.Fatalf("read conflicted local Query Policy: %v", err)
	}
	if status != "DEPRECATED" {
		t.Fatalf("fixture reactivated an existing Deprecated Policy: %s", status)
	}
}

func TestLocalManagedTableFixtureRejectsIncompatibleExistingTableWithoutAssignment(t *testing.T) {
	ctx, driverConfig := startCurrentIntegrationMySQL(t, "testdata/009-incompatible-local-fixture.sql")
	if err := executeLocalManagedTableFixture(driverConfig); err == nil {
		t.Fatal("expected incompatible existing notification_templates schema to reject fixture application")
	}
	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open incompatible fixture verification database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	var assignments int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM rcc_table_policies WHERE table_name = 'notification_templates'`).Scan(&assignments); err != nil {
		t.Fatalf("count incompatible local Table Policy assignments: %v", err)
	}
	if assignments != 0 {
		t.Fatalf("fixture assigned incompatible notification_templates table: %d", assignments)
	}
}

func applyLocalManagedTableFixture(t *testing.T, driverConfig *mysqldriver.Config) {
	t.Helper()
	if err := executeLocalManagedTableFixture(driverConfig); err != nil {
		t.Fatalf("apply local Managed Table fixture a second time: %v", err)
	}
}

func executeLocalManagedTableFixture(driverConfig *mysqldriver.Config) error {
	fixture, err := os.ReadFile(localManagedTableFixture)
	if err != nil {
		return err
	}
	repeatConfig := *driverConfig
	repeatConfig.MultiStatements = true
	database, err := sql.Open("mysql", repeatConfig.FormatDSN())
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	_, err = database.Exec(string(fixture))
	return err
}

func assertLocalFixturePolicies(t *testing.T, app *adminApplication) {
	t.Helper()
	tablePolicy := policyIntegrationRequest(t, app, http.MethodGet, "/api/v1/table-policies/notification_templates", "")
	if tablePolicy.Code != http.StatusOK {
		t.Fatalf("read fixture Table Policy: HTTP %d %s", tablePolicy.Code, tablePolicy.Body.String())
	}
	var assignment struct {
		QueryPolicyCode    string `json:"query_policy_code"`
		MutationPolicyCode string `json:"mutation_policy_code"`
		Enabled            bool   `json:"enabled"`
	}
	decodeFixtureResponse(t, tablePolicy.Body.Bytes(), &assignment)
	if assignment.QueryPolicyCode != "notification_page_query_v1" || assignment.MutationPolicyCode != "notification_full_mutation_v1" || !assignment.Enabled {
		t.Fatalf("unexpected local Policy Snapshot assignment: %s", tablePolicy.Body.String())
	}

	queryPolicy := policyIntegrationRequest(t, app, http.MethodGet, "/api/v1/query-policies/notification_page_query_v1", "")
	if queryPolicy.Code != http.StatusOK {
		t.Fatalf("read fixture Query Policy: HTTP %d %s", queryPolicy.Code, queryPolicy.Body.String())
	}
	var queryDefinition struct {
		Status string `json:"status"`
	}
	decodeFixtureResponse(t, queryPolicy.Body.Bytes(), &queryDefinition)
	if queryDefinition.Status != "ACTIVE" {
		t.Fatalf("fixture Query Policy is not Active: %s", queryPolicy.Body.String())
	}

	mutationPolicy := policyIntegrationRequest(t, app, http.MethodGet, "/api/v1/mutation-policies/notification_full_mutation_v1", "")
	if mutationPolicy.Code != http.StatusOK {
		t.Fatalf("read fixture Mutation Policy: HTTP %d %s", mutationPolicy.Code, mutationPolicy.Body.String())
	}
	var mutationDefinition struct {
		Status      string `json:"status"`
		AllowAdd    bool   `json:"allow_add"`
		AllowModify bool   `json:"allow_modify"`
		AllowDelete bool   `json:"allow_delete"`
	}
	decodeFixtureResponse(t, mutationPolicy.Body.Bytes(), &mutationDefinition)
	if mutationDefinition.Status != "ACTIVE" || !mutationDefinition.AllowAdd || !mutationDefinition.AllowModify || !mutationDefinition.AllowDelete {
		t.Fatalf("fixture Mutation Policy is not fully Active: %s", mutationPolicy.Body.String())
	}
}

func queryNotificationTemplate(t *testing.T, app *adminApplication, templateKey string) map[string]*string {
	t.Helper()
	rows := queryNotificationTemplateRows(t, app, templateKey)
	if len(rows) != 1 {
		t.Fatalf("expected one notification template %q, got %d", templateKey, len(rows))
	}
	return rows[0]
}

func assertNotificationTemplateAbsent(t *testing.T, app *adminApplication, templateKey string) {
	t.Helper()
	if rows := queryNotificationTemplateRows(t, app, templateKey); len(rows) != 0 {
		t.Fatalf("notification template %q was not cleaned up: %#v", templateKey, rows)
	}
}

func queryNotificationTemplateRows(t *testing.T, app *adminApplication, templateKey string) []map[string]*string {
	t.Helper()
	response := policyIntegrationRequest(t, app, http.MethodPost, "/api/v1/tables/notification_templates/query", `{
		"conditions":[{"field":"template_key","operator":"exact","value":"`+templateKey+`"}]
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("query notification template %q: HTTP %d %s", templateKey, response.Code, response.Body.String())
	}
	var result struct {
		Rows []map[string]*string `json:"rows"`
	}
	decodeFixtureResponse(t, response.Body.Bytes(), &result)
	return result.Rows
}

func decodeFixtureResponse(t *testing.T, payload []byte, destination any) {
	t.Helper()
	if err := json.Unmarshal(payload, destination); err != nil {
		t.Fatalf("decode fixture response: %v", err)
	}
}
