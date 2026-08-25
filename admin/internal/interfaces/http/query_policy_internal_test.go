package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestQueryPolicyResponseUsesCatalogAuditFieldNames(t *testing.T) {
	now := time.Date(2026, time.August, 24, 12, 30, 0, 0, time.UTC)
	encoded, err := json.Marshal(queryPolicyResponseFor(domain.QueryPolicy{
		Code: "standard_page_query_v1", CreatedAt: now, UpdatedAt: now,
	}))
	if err != nil {
		t.Fatalf("marshal Query Policy response: %v", err)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("decode Query Policy response: %v", err)
	}
	for _, required := range []string{"gmt_created", "gmt_modified"} {
		if _, found := response[required]; !found {
			t.Fatalf("Query Policy response is missing %s: %s", required, encoded)
		}
	}
	for _, forbidden := range []string{"created_at", "updated_at"} {
		if _, found := response[forbidden]; found {
			t.Fatalf("Query Policy response exposed legacy-incompatible field %s: %s", forbidden, encoded)
		}
	}
}
