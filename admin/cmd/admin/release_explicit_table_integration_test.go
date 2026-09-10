//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestReleaseWritesRequireExplicitTableOnEveryDetail(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	reviewer := publicationFixtureReviewer(t, app)
	for index, action := range []string{"incremental", "copy", "reprepare"} {
		t.Run(action, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"title": "explicit table", "items": []any{map[string]any{"table_name": "mutation_add_items", "operation": "ADD", "content": map[string]string{"id": fmt.Sprint(index + 101), "code": "required-table-" + action, "label": "original"}}}})
			var source domain.ReleaseOrder
			var path string
			if action == "reprepare" {
				path = approvePublication(t, app, reviewer, string(body), "required-table-"+action)
				source = rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
			} else {
				source = rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), "required-table-"+action), 201)
				path = "/api/v1/release-orders/" + source.ID
				if action == "copy" {
					source = rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"1","reason":"copy fixture"}`, "required-table-cancel"), 200)
				}
			}
			snapshots := map[string]string{}
			for _, table := range []string{"rcc_release_orders", "rcc_release_details", "rcc_release_executions", "rcc_release_requests", "rcc_release_targets", "rcc_release_table_references", "rcc_publication_commands", "rcc_refresh_notifications"} {
				snapshots[table] = baselineRows(t, db, "SELECT * FROM "+table+" ORDER BY 1,2")
			}
			item := map[string]any{"detail_id": source.Items[0].DetailID, "operation": "ADD", "expected_record_version": "0", "content": source.Items[0].Content}
			input := map[string]any{"expected_version": source.Version, "confirmed": true, "items": []any{item}}
			method, target := "POST", path+"/"+action
			if action == "incremental" {
				method, target = "PUT", path
				input = map[string]any{"title": source.Title, "expected_version": source.Version, "changes": map[string]any{"upserts": []any{item}}}
			}
			missingTable, _ := json.Marshal(input)
			assertIntegrationErrorCode(t, releaseRequest(t, app, method, target, string(missingTable), "required-table-missing-"+action), 422, "release_invalid")
			for table, before := range snapshots {
				if after := baselineRows(t, db, "SELECT * FROM "+table+" ORDER BY 1,2"); after != before {
					t.Fatalf("missing detail table changed %s", table)
				}
			}
		})
	}
}
