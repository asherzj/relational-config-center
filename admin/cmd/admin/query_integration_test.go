//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryPolicyContainsTreatsWildcardsAndEscapeCharacterLiterally(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	enableQueryPolicy(t, app, "query_policy_items", `{}`)

	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", `{
		"conditions":[{"field":"name","operator":"contains","value":"%_!"}],
		"order":{"field":"id","direction":"ASC"},
		"page_number":1,
		"page_size":20
	}`)
	assertQueryIDs(t, response, "1")
}

func TestQueryPolicyAppliesOpenAndClosedRanges(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	enableQueryPolicy(t, app, "query_policy_items", `{}`)

	tests := []struct {
		name     string
		body     string
		expected []string
	}{
		{
			name:     "open range excludes both boundaries",
			body:     `{"conditions":[{"field":"score","operator":"open_range","from":"10","to":"30"}],"order":{"field":"id","direction":"ASC"}}`,
			expected: []string{"2"},
		},
		{
			name:     "closed range includes both boundaries",
			body:     `{"conditions":[{"field":"score","operator":"closed_range","from":"10","to":"30"}],"order":{"field":"id","direction":"ASC"}}`,
			expected: []string{"1", "2", "3"},
		},
		{
			name:     "one open boundary is enough",
			body:     `{"conditions":[{"field":"score","operator":"open_range","from":"30"}],"order":{"field":"id","direction":"ASC"}}`,
			expected: []string{"4"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertQueryIDs(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", test.body), test.expected...)
		})
	}

	missingBoundary := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", `{"conditions":[{"field":"score","operator":"open_range"}]}`)
	assertIntegrationErrorCode(t, missingBoundary, http.StatusBadRequest, "invalid_query_condition")
}

func TestQueryPolicyAppliesMembershipNullAndEmptyStringSemantics(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	enableQueryPolicy(t, app, "query_policy_items", `{}`)

	tests := []struct {
		name     string
		body     string
		expected []string
	}{
		{name: "in", body: `{"conditions":[{"field":"score","operator":"in","values":["10","30"]}],"order":{"field":"id","direction":"ASC"}}`, expected: []string{"1", "3"}},
		{name: "not in", body: `{"conditions":[{"field":"score","operator":"not_in","values":["10","30"]}],"order":{"field":"id","direction":"ASC"}}`, expected: []string{"2", "4"}},
		{name: "is null", body: `{"conditions":[{"field":"nullable_value","operator":"is_null"}],"order":{"field":"id","direction":"ASC"}}`, expected: []string{"3"}},
		{name: "is not null", body: `{"conditions":[{"field":"nullable_value","operator":"is_not_null"}],"order":{"field":"id","direction":"ASC"}}`, expected: []string{"1", "2", "4"}},
		{name: "empty string is data", body: `{"conditions":[{"field":"name","operator":"exact","value":""}],"order":{"field":"id","direction":"ASC"}}`, expected: []string{"3"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertQueryIDs(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", test.body), test.expected...)
		})
	}

	invalidConditions := []string{
		`{"conditions":[{"field":"score","operator":"in","values":[]}]}`,
		`{"conditions":[{"field":"score","operator":"not_in"}]}`,
		`{"conditions":[{"field":"score","operator":"in","values":[null]}]}`,
		`{"conditions":[{"field":"nullable_value","operator":"exact","value":null}]}`,
		`{"conditions":[{"field":"score","operator":"open_range","from":null,"to":"30"}]}`,
		`{"conditions":[{"field":"nullable_value","operator":"is_null","value":null}]}`,
		`{"conditions":[{"field":"nullable_value","operator":"is_null","value":""}]}`,
	}
	for _, body := range invalidConditions {
		response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", body)
		assertIntegrationErrorCode(t, response, http.StatusBadRequest, "invalid_query_condition")
	}
}

func TestQueryPolicyEnforcesLimitsAndRequestSortWithoutCorrection(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	enableQueryPolicy(t, app, "query_policy_items", `{"default_order":{"field":"id","direction":"DESC"}}`)

	condition := `{"field":"score","operator":"closed_range","from":"10"}`
	twentyConditions := `{"conditions":[` + strings.Join(repeated(condition, 20), ",") + `],"page_size":200}`
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", twentyConditions); response.Code != http.StatusOK {
		t.Fatalf("20 conditions must be accepted: HTTP %d %s", response.Code, response.Body.String())
	}
	twentyOneConditions := `{"conditions":[` + strings.Join(repeated(condition, 21), ",") + `]}`
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", twentyOneConditions), http.StatusBadRequest, "invalid_query_condition")

	hundredValues := `{"conditions":[{"field":"score","operator":"in","values":[` + strings.Join(repeated(`"10"`, 100), ",") + `]}]}`
	assertQueryIDs(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", hundredValues), "1")
	hundredOneValues := `{"conditions":[{"field":"score","operator":"in","values":[` + strings.Join(repeated(`"10"`, 101), ",") + `]}]}`
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", hundredOneValues), http.StatusBadRequest, "invalid_query_condition")

	maximumOffset := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", `{"page_number":51,"page_size":200}`)
	if maximumOffset.Code != http.StatusOK {
		t.Fatalf("offset 10000 must be accepted: HTTP %d %s", maximumOffset.Code, maximumOffset.Body.String())
	}
	for _, body := range []string{
		`{"page_size":201}`,
		`{"page_number":52,"page_size":200}`,
		`{"page_number":9223372036854775807,"page_size":200}`,
	} {
		assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", body), http.StatusBadRequest, "invalid_pagination")
	}

	assertQueryIDs(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", `{"page_size":1}`), "4")
	assertQueryIDs(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", `{"order":{"field":"id","direction":"ASC"},"page_size":1}`), "1")
	for _, body := range []string{
		`{"order":{"field":"id","direction":"asc"}}`,
		`{"order":{"field":"missing","direction":"ASC"}}`,
		`{"order":{"field":"id` + "`" + ` DESC, score","direction":"ASC"}}`,
	} {
		assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", body), http.StatusBadRequest, "invalid_query_order")
	}
}

func TestQueryPolicyReturnsEverySupportedLiveTypeAsLosslessJSONString(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	enableQueryPolicy(t, app, "query_type_values", `{}`)

	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_type_values/query", `{"page_size":1}`)
	if response.Code != http.StatusOK {
		t.Fatalf("query supported live types: HTTP %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode supported type response: %v", err)
	}
	if len(body.Rows) != 1 {
		t.Fatalf("expected one row, got %s", response.Body.String())
	}
	columnTypes := make(map[string]string, len(body.Columns))
	for _, column := range body.Columns {
		columnTypes[column.Name] = column.Type
	}
	expectedTypes := map[string]string{
		"id": "uint64", "signed_tiny": "int64", "unsigned_int": "uint64",
		"signed_big": "int64", "unsigned_big": "uint64", "decimal_value": "decimal",
		"float_value": "float64", "double_value": "float64", "char_value": "string",
		"varchar_value": "string", "text_value": "string", "enum_value": "string",
		"boolean_value": "boolean", "date_value": "date", "time_value": "time",
		"datetime_value": "datetime", "timestamp_value": "timestamp", "json_value": "json",
		"nullable_value": "string",
	}
	for name, expected := range expectedTypes {
		if columnTypes[name] != expected {
			t.Fatalf("column %s: expected type %s, got %s", name, expected, columnTypes[name])
		}
	}

	row := body.Rows[0]
	expectedValues := map[string]string{
		"id": "1", "signed_tiny": "-128", "unsigned_int": "4294967295",
		"signed_big": "-9223372036854775808", "unsigned_big": "18446744073709551615",
		"decimal_value": "123456789012345678901234567890.123456789012345678901234567890",
		"float_value":   "1.5", "double_value": "1.5", "char_value": "char",
		"varchar_value": "varchar", "text_value": "text", "enum_value": "alpha",
		"boolean_value": "1", "date_value": "2024-02-29", "time_value": "23:59:58.123456",
		"datetime_value":  "2024-02-29 23:59:58.123456",
		"timestamp_value": "2024-02-29T23:59:58.123456Z",
	}
	for name, expected := range expectedValues {
		if actual := value(row[name]); actual != expected {
			t.Fatalf("column %s: expected value %q, got %q", name, expected, actual)
		}
	}
	if row["nullable_value"] != nil {
		t.Fatalf("SQL NULL was not preserved: %s", response.Body.String())
	}
	jsonValue := value(row["json_value"])
	if !json.Valid([]byte(jsonValue)) || !strings.Contains(jsonValue, "9007199254740993") {
		t.Fatalf("JSON value was not returned losslessly: %q", jsonValue)
	}
}

func TestQueryPolicyParsesConditionValuesByLiveType(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	enableQueryPolicy(t, app, "query_type_values", `{}`)

	valid := []struct {
		field string
		value string
	}{
		{field: "signed_big", value: "-9223372036854775808"},
		{field: "unsigned_big", value: "18446744073709551615"},
		{field: "decimal_value", value: "123456789012345678901234567890.123456789012345678901234567890"},
		{field: "float_value", value: "1.5"},
		{field: "varchar_value", value: "varchar"},
		{field: "enum_value", value: "alpha"},
		{field: "boolean_value", value: "1"},
		{field: "date_value", value: "2024-02-29"},
		{field: "time_value", value: "23:59:58.123456"},
		{field: "datetime_value", value: "2024-02-29 23:59:58.123456"},
		{field: "timestamp_value", value: "2024-02-29T23:59:58.123456Z"},
		{field: "json_value", value: `{"nested":{"n":9007199254740993},"ok":true}`},
	}
	for _, item := range valid {
		t.Run(item.field, func(t *testing.T) {
			valueJSON, err := json.Marshal(item.value)
			if err != nil {
				t.Fatalf("encode condition value: %v", err)
			}
			body := `{"conditions":[{"field":"` + item.field + `","operator":"exact","value":` + string(valueJSON) + `}]}`
			assertQueryIDs(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_type_values/query", body), "1")
		})
	}

	invalid := []struct {
		field    string
		operator string
		value    string
	}{
		{field: "signed_big", operator: "exact", value: "9223372036854775808"},
		{field: "unsigned_big", operator: "exact", value: "-1"},
		{field: "decimal_value", operator: "exact", value: "1e3"},
		{field: "float_value", operator: "exact", value: "NaN"},
		{field: "boolean_value", operator: "exact", value: "true"},
		{field: "date_value", operator: "exact", value: "2023-02-29"},
		{field: "time_value", operator: "exact", value: "24:00:00"},
		{field: "datetime_value", operator: "exact", value: "2024-02-29T23:59:58Z"},
		{field: "timestamp_value", operator: "exact", value: "2024-02-29 23:59:58"},
		{field: "json_value", operator: "exact", value: "{broken"},
		{field: "signed_big", operator: "contains", value: "22"},
		{field: "json_value", operator: "open_range", value: "{}"},
	}
	for _, item := range invalid {
		t.Run("invalid "+item.field+" "+item.operator, func(t *testing.T) {
			valueJSON, err := json.Marshal(item.value)
			if err != nil {
				t.Fatalf("encode invalid condition value: %v", err)
			}
			var body string
			if item.operator == "open_range" {
				body = `{"conditions":[{"field":"` + item.field + `","operator":"` + item.operator + `","from":` + string(valueJSON) + `}]}`
			} else {
				body = `{"conditions":[{"field":"` + item.field + `","operator":"` + item.operator + `","value":` + string(valueJSON) + `}]}`
			}
			assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_type_values/query", body), http.StatusBadRequest, "invalid_query_condition")
		})
	}
}

func TestQueryPolicyRejectsEveryUnsupportedFullRowType(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableQueryPolicy(t, app, "query_policy_items", `{}`)

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	unsupported := []struct {
		name       string
		definition string
	}{
		{name: "binary_value", definition: "BINARY(8) NULL"},
		{name: "varbinary_value", definition: "VARBINARY(8) NULL"},
		{name: "blob_value", definition: "BLOB NULL"},
		{name: "bit_value", definition: "BIT(8) NULL"},
		{name: "set_value", definition: "SET('a','b') NULL"},
		{name: "spatial_value", definition: "POINT NULL"},
	}
	for _, item := range unsupported {
		t.Run(item.name, func(t *testing.T) {
			if _, err := database.ExecContext(ctx, "ALTER TABLE `query_policy_items` ADD COLUMN `"+item.name+"` "+item.definition); err != nil {
				t.Fatalf("add unsupported live column: %v", err)
			}
			response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", `{}`)
			assertIntegrationErrorCode(t, response, http.StatusUnprocessableEntity, "incompatible_table")
			if _, err := database.ExecContext(ctx, "ALTER TABLE `query_policy_items` DROP COLUMN `"+item.name+"`"); err != nil {
				t.Fatalf("remove unsupported live column: %v", err)
			}
		})
	}
}

func TestQueryPolicyMapsDatabaseTimeoutToSafeGatewayTimeout(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/005-query-policy-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableQueryPolicy(t, app, "query_policy_items", `{}`)

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open locking database connection: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	connection, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve locking database connection: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if _, err := connection.ExecContext(ctx, "LOCK TABLES `query_policy_items` WRITE"); err != nil {
		t.Fatalf("lock Managed Table: %v", err)
	}
	t.Cleanup(func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.ExecContext(unlockContext, "UNLOCK TABLES")
	})

	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_policy_items/query", `{}`)
	assertIntegrationErrorCode(t, response, http.StatusGatewayTimeout, "query_timeout")
	if strings.Contains(response.Body.String(), "query_policy_items") || strings.Contains(response.Body.String(), "SELECT") {
		t.Fatalf("timeout response exposed storage details: %s", response.Body.String())
	}
}

func repeated(value string, count int) []string {
	result := make([]string, count)
	for index := range result {
		result[index] = value
	}
	return result
}

func TestEnabledTablePolicyQueriesExactRowsWithPolicyDefaults(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/004-query-fixture.sql",
	)

	policy := integrationPolicyPayload(
		"query_items",
		"mysql_page_query_v1",
		`{"default_order":{"field":"id","direction":"DESC"},"default_page_size":2,"max_page_size":5}`,
		"mysql_single_table_mutation_v1",
		`{}`,
	)
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", policy)
	if created.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", created.Code, created.Body.String())
	}
	enabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/query_items/enable", "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", enabled.Code, enabled.Body.String())
	}

	queried := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{"conditions":[{"field":"category","operator":"exact","value":"alpha"}],"page_number":1}`)
	if queried.Code != http.StatusOK {
		t.Fatalf("query Managed Table: HTTP %d %s", queried.Code, queried.Body.String())
	}

	var response struct {
		Columns []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Nullable bool   `json:"nullable"`
		} `json:"columns"`
		Rows []map[string]*string `json:"rows"`
		Page struct {
			PageNumber int   `json:"page_number"`
			PageSize   int   `json:"page_size"`
			TotalCount int64 `json:"total_count"`
			TotalPages int64 `json:"total_pages"`
		} `json:"page"`
	}
	if err := json.Unmarshal(queried.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(response.Columns) != 3 || response.Columns[0].Name != "id" || response.Columns[0].Type != "uint64" || response.Columns[0].Nullable {
		t.Fatalf("unexpected live column metadata: %#v", response.Columns)
	}
	if len(response.Rows) != 2 || value(response.Rows[0]["id"]) != "4" || response.Rows[0]["label"] != nil || value(response.Rows[1]["id"]) != "3" {
		t.Fatalf("unexpected JSON-string/null rows: %#v", response.Rows)
	}
	if response.Page.PageNumber != 1 || response.Page.PageSize != 2 || response.Page.TotalCount != 3 || response.Page.TotalPages != 2 {
		t.Fatalf("unexpected page metadata: %#v", response.Page)
	}
}

func TestPolicyReplacementAndDisableAffectTheNextQuery(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/004-query-fixture.sql",
	)

	policy := integrationPolicyPayload(
		"query_items",
		"mysql_page_query_v1",
		`{"default_order":{"field":"id","direction":"DESC"},"default_page_size":1,"max_page_size":5}`,
		"mysql_single_table_mutation_v1",
		`{}`,
	)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", policy); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/query_items/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	assertFirstQueryID(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{}`), "4")

	replacement := integrationPolicyPayload(
		"query_items",
		"mysql_page_query_v1",
		`{"default_order":{"field":"id","direction":"ASC"},"default_page_size":1,"max_page_size":5}`,
		"mysql_single_table_mutation_v1",
		`{}`,
	)
	replaced := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/query_items", replacement)
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace Policy: HTTP %d %s", replaced.Code, replaced.Body.String())
	}
	assertFirstQueryID(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{}`), "1")

	invalid := integrationPolicyPayload(
		"query_items",
		"mysql_page_query_v1",
		`{"default_order":{"field":"missing_column","direction":"DESC"},"default_page_size":1,"max_page_size":5}`,
		"mysql_single_table_mutation_v1",
		`{}`,
	)
	rejected := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/query_items", invalid)
	assertIntegrationErrorCode(t, rejected, http.StatusUnprocessableEntity, "invalid_policy_config")
	assertFirstQueryID(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{}`), "1")

	disabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/query_items/disable", "")
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable Policy: HTTP %d %s", disabled.Code, disabled.Body.String())
	}
	denied := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{}`)
	assertIntegrationErrorCode(t, denied, http.StatusForbidden, "table_policy_disabled")
}

func TestExactQueryUsesANDValidatedSortAndPreservesAnEmptyRequestedPage(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/004-query-fixture.sql",
	)
	policy := integrationPolicyPayload("query_items", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", policy); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/query_items/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	andQuery := `{"conditions":[{"field":"category","operator":"exact","value":"alpha"},{"field":"label","operator":"exact","value":"third"}],"order":{"field":"id","direction":"ASC"},"page_number":1,"page_size":10}`
	andResult := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", andQuery)
	assertFirstQueryID(t, andResult, "3")
	var matched struct {
		Rows []map[string]*string `json:"rows"`
		Page struct {
			TotalCount int64 `json:"total_count"`
		} `json:"page"`
	}
	if err := json.Unmarshal(andResult.Body.Bytes(), &matched); err != nil {
		t.Fatalf("decode AND query: %v", err)
	}
	if len(matched.Rows) != 1 || matched.Page.TotalCount != 1 {
		t.Fatalf("conditions were not AND-connected: %s", andResult.Body.String())
	}

	empty := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{"conditions":[{"field":"category","operator":"exact","value":"missing"}],"page_number":7,"page_size":2}`)
	if empty.Code != http.StatusOK {
		t.Fatalf("query empty page: HTTP %d %s", empty.Code, empty.Body.String())
	}
	var emptyPage struct {
		Rows []map[string]*string `json:"rows"`
		Page struct {
			PageNumber int   `json:"page_number"`
			PageSize   int   `json:"page_size"`
			TotalCount int64 `json:"total_count"`
			TotalPages int64 `json:"total_pages"`
		} `json:"page"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyPage); err != nil {
		t.Fatalf("decode empty query: %v", err)
	}
	if len(emptyPage.Rows) != 0 || emptyPage.Page.PageNumber != 7 || emptyPage.Page.PageSize != 2 || emptyPage.Page.TotalCount != 0 || emptyPage.Page.TotalPages != 0 {
		t.Fatalf("empty result did not preserve the requested valid page: %s", empty.Body.String())
	}

	invalidSort := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{"order":{"field":"missing","direction":"ASC"}}`)
	assertIntegrationErrorCode(t, invalidSort, http.StatusBadRequest, "invalid_query_order")

	boundValue := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{"conditions":[{"field":"category","operator":"exact","value":"alpha' OR 1=1 --"}]}`)
	if boundValue.Code != http.StatusOK {
		t.Fatalf("query bound value: HTTP %d %s", boundValue.Code, boundValue.Body.String())
	}
	var boundResponse struct {
		Rows []map[string]*string `json:"rows"`
		Page struct {
			TotalCount int64 `json:"total_count"`
		} `json:"page"`
	}
	if err := json.Unmarshal(boundValue.Body.Bytes(), &boundResponse); err != nil {
		t.Fatalf("decode bound-value query: %v", err)
	}
	if len(boundResponse.Rows) != 0 || boundResponse.Page.TotalCount != 0 {
		t.Fatalf("exact value was not bound as data: %s", boundValue.Body.String())
	}
	containsInjection := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{"conditions":[{"field":"category","operator":"contains","value":"%' OR 1=1 --"}]}`)
	if containsInjection.Code != http.StatusOK {
		t.Fatalf("query contains injection-shaped value: HTTP %d %s", containsInjection.Code, containsInjection.Body.String())
	}
	if strings.Contains(containsInjection.Body.String(), "alpha") {
		t.Fatalf("contains value changed query structure: %s", containsInjection.Body.String())
	}

	invalidIdentifier := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{"conditions":[{"field":"category`+"`"+` OR 1=1","operator":"exact","value":"alpha"}]}`)
	assertIntegrationErrorCode(t, invalidIdentifier, http.StatusBadRequest, "invalid_query_condition")
	if strings.Contains(invalidIdentifier.Body.String(), "OR 1=1") || strings.Contains(invalidIdentifier.Body.String(), "category") {
		t.Fatalf("safe error exposed the rejected identifier: %s", invalidIdentifier.Body.String())
	}
}

func TestQueryFailsClosedWhenLiveSchemaBecomesInvalid(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/004-query-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	policy := integrationPolicyPayload("query_items", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", policy); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/query_items/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "ALTER TABLE `query_items` ADD COLUMN `unsupported_payload` BLOB NULL"); err != nil {
		t.Fatalf("drift live Schema: %v", err)
	}

	rejected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{}`)
	assertIntegrationErrorCode(t, rejected, http.StatusUnprocessableEntity, "incompatible_table")
}

func TestQueryFailsClosedWhenPolicyCatalogIsUnavailable(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/004-query-fixture.sql",
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
	if _, err := database.ExecContext(ctx, "DROP TABLE `rcc_table_policies`"); err != nil {
		t.Fatalf("make Policy Catalog unavailable: %v", err)
	}

	rejected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/query_items/query", `{}`)
	assertIntegrationErrorCode(t, rejected, http.StatusServiceUnavailable, "policy_catalog_unavailable")
}

func assertFirstQueryID(t *testing.T, response *httptest.ResponseRecorder, expected string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("query Managed Table: HTTP %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(body.Rows) == 0 || value(body.Rows[0]["id"]) != expected {
		t.Fatalf("expected first id %s, got %s", expected, response.Body.String())
	}
}

func value(cell *string) string {
	if cell == nil {
		return ""
	}
	return *cell
}

func enableQueryPolicy(t *testing.T, app *adminApplication, tableName, queryConfig string) {
	t.Helper()
	policy := integrationPolicyPayload(tableName, "mysql_page_query_v1", queryConfig, "mysql_single_table_mutation_v1", `{}`)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", policy); response.Code != http.StatusCreated {
		t.Fatalf("create Table Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/"+tableName+"/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Table Policy: HTTP %d %s", response.Code, response.Body.String())
	}
}

func assertQueryIDs(t *testing.T, response *httptest.ResponseRecorder, expected ...string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("query Managed Table: HTTP %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	actual := make([]string, 0, len(body.Rows))
	for _, row := range body.Rows {
		actual = append(actual, value(row["id"]))
	}
	if strings.Join(actual, ",") != strings.Join(expected, ",") {
		t.Fatalf("expected ids %v, got %v: %s", expected, actual, response.Body.String())
	}
}
