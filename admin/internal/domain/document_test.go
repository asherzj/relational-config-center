package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// The JSON tags define the persisted policy-document format; a round trip must
// preserve every policy attribute without loss or renames.
func TestPolicyDocumentRoundTrip(t *testing.T) {
	original := testPolicy()
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Policy
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded policy no longer validates: %v", err)
	}
	if decoded.Fields["name"].MaxLength != original.Fields["name"].MaxLength ||
		decoded.Fields["id"].AutoIncrement != original.Fields["id"].AutoIncrement ||
		!decoded.AllowDelete || decoded.DefaultPageSize != 0 {
		t.Fatalf("round trip lost policy attributes: %#v", decoded)
	}
}

// Strict decoding means a typo in a hand-edited catalog row fails loudly at
// startup instead of silently dropping a constraint.
func TestPolicyDocumentRejectsUnknownFields(t *testing.T) {
	decoder := json.NewDecoder(strings.NewReader(`{"resource":"widgets","table":"app_widgets","primary_key":"id","felds":{}}`))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err == nil {
		t.Fatal("Decode() error = nil, want unknown-field error")
	}
}
