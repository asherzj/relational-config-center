//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCombinedQueryAC016RealFieldsCapacityAndAtomicConfiguration(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	db := deliveryDB(t, driver)
	columns := []string{"id bigint unsigned NOT NULL PRIMARY KEY"}
	for i := 1; i <= 256; i++ {
		columns = append(columns, fmt.Sprintf("f%d int NOT NULL DEFAULT 1", i))
	}
	deliveryExec(t, db, "CREATE TABLE wide_items ("+strings.Join(columns, ",")+")")
	deliveryExec(t, db, "INSERT INTO wide_items (id) VALUES (1),(2)")
	deliveryExec(t, db, "UPDATE wide_items SET f255=2 WHERE id=2")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	enableQueryPolicy(t, app, "wide_items", queryPolicyFixture{DefaultOrderField: "id", DefaultOrderDirection: "ASC"})
	path := "/api/v1/table-field-policies/wide_items"
	read := policyIntegrationRequest(t, app, "GET", path, "")
	var capacity struct {
		QueryCapacity struct {
			MaxConditions int  `json:"max_conditions"`
			MaxValues     int  `json:"max_values_per_condition"`
			Count         int  `json:"queryable_fields"`
			Supported     bool `json:"supported"`
		} `json:"query_capacity"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &capacity); err != nil {
		t.Fatal(err)
	}
	if read.Code != 200 || capacity.QueryCapacity.MaxConditions != 256 || capacity.QueryCapacity.MaxValues != 100 || capacity.QueryCapacity.Count != 257 || capacity.QueryCapacity.Supported {
		t.Fatalf("missing real-field capacity: %d %s", read.Code, read.Body.String())
	}
	reject := policyIntegrationRequest(t, app, "PUT", path, `{"policies":[]}`)
	assertIntegrationErrorCode(t, reject, 422, "invalid_field_policy")
	// Only one configured row: the remaining 256 missing rules must still count.
	const policy = `{"policies":[{"field_name":"f256","display_name":"最后字段","is_visible":true,"is_queryable":false,"query_operators":[],"ui_type":"text","editable_on_add":true,"editable_on_modify":true,"enabled":true}]}`
	saved := policyIntegrationRequest(t, app, "PUT", path, policy)
	if saved.Code != 200 {
		t.Fatalf("reduce effective queryable fields: %d %s", saved.Code, saved.Body.String())
	}
	json.Unmarshal(saved.Body.Bytes(), &capacity)
	if capacity.QueryCapacity.Count != 256 || !capacity.QueryCapacity.Supported {
		t.Fatalf("wrong saved capacity: %s", saved.Body.String())
	}
	conditions := []string{`{"field":"id","operator":"closed_range","from":"1","to":"2"}`}
	for i := 1; i <= 255; i++ {
		conditions = append(conditions, fmt.Sprintf(`{"field":"f%d","operator":"exact","value":"1"}`, i))
	}
	assertQueryIDs(t, policyIntegrationRequest(t, app, "POST", "/api/v1/tables/wide_items/query", `{"conditions":[`+strings.Join(conditions, ",")+`]}`), "1")
	// Worst-case IN parameter shape still fits the retained body and DB limits.
	for i := range conditions {
		name := "id"
		if i > 0 {
			name = fmt.Sprintf("f%d", i)
		}
		conditions[i] = fmt.Sprintf(`{"field":"%s","operator":"in","values":[%s]}`, name, strings.Join(repeated(`"1"`, 100), ","))
	}
	assertQueryIDs(t, policyIntegrationRequest(t, app, "POST", "/api/v1/tables/wide_items/query", `{"conditions":[`+strings.Join(conditions, ",")+`]}`), "1")
	rejected := policyIntegrationRequest(t, app, "PUT", path, strings.Replace(policy, `"enabled":true`, `"enabled":false`, 1))
	assertIntegrationErrorCode(t, rejected, 422, "invalid_field_policy")
	after := policyIntegrationRequest(t, app, "GET", path, "")
	if after.Body.String() != saved.Body.String() {
		t.Fatal("capacity rejection changed configuration")
	}
}
