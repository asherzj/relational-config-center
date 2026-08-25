package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestMutationPolicyResponseUsesTypedRelationalFieldsAndCatalogAuditNames(t *testing.T) {
	now := time.Date(2026, time.August, 24, 12, 30, 0, 0, time.UTC)
	creator := "creator"
	encoded, err := json.Marshal(mutationPolicyResponseFor(domain.MutationPolicy{
		Code: "standard_mutation_v1", CreateOperatorField: &creator, CreatedAt: now, UpdatedAt: now,
	}))
	if err != nil {
		t.Fatalf("marshal Mutation Policy response: %v", err)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("decode Mutation Policy response: %v", err)
	}
	for _, required := range []string{"allow_add", "allow_modify", "allow_delete", "create_operator_field", "create_time_field", "modify_operator_field", "modify_time_field", "gmt_created", "gmt_modified"} {
		if _, found := response[required]; !found {
			t.Fatalf("Mutation Policy response is missing %s: %s", required, encoded)
		}
	}
	for _, forbidden := range []string{"config", "auto_fill", "supports_add", "supports_modify", "supports_delete", "created_at", "updated_at"} {
		if _, found := response[forbidden]; found {
			t.Fatalf("Mutation Policy response exposed forbidden field %s: %s", forbidden, encoded)
		}
	}
}
