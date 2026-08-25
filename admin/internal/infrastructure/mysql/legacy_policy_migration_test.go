package mysql

import (
	"strings"
	"testing"
)

func TestLegacyBackfillNormalizesDefaultsAndProducesStableTechnologyNeutralCodes(t *testing.T) {
	record := legacyPolicyRecord{
		Table: "feature_flags", QueryPolicy: "mysql_page_query_v1", QueryPolicyConfig: `{}`,
		MutationPolicy: "mysql_single_table_mutation_v1", MutationPolicyConfig: `{}`,
		AllowAdd: true, AllowModify: false, AllowDelete: true,
	}
	first, problems := planLegacyBackfill(record)
	if len(problems) != 0 {
		t.Fatalf("canonical legacy Policy was rejected: %v", problems)
	}
	second, problems := planLegacyBackfill(record)
	if len(problems) != 0 {
		t.Fatalf("repeated planning failed: %v", problems)
	}
	if first.query.Code != second.query.Code || first.mutation.Code != second.mutation.Code {
		t.Fatalf("deterministic Codes changed: first=%s/%s second=%s/%s", first.query.Code, first.mutation.Code, second.query.Code, second.mutation.Code)
	}
	if first.query.DefaultOrderField != "id" || first.query.DefaultOrderDirection != "DESC" || first.query.DefaultPageSize != 20 || first.query.MaxPageSize != 200 {
		t.Fatalf("legacy query defaults were not normalized: %#v", first.query)
	}
	for _, code := range []string{first.query.Code, first.mutation.Code} {
		if strings.HasPrefix(code, "mysql_") || !strings.HasSuffix(code, "_v1") {
			t.Fatalf("backfill Code is not technology-neutral/versioned: %s", code)
		}
	}
}

func TestLegacyBackfillMapsOnlyStandardAuditAutoFill(t *testing.T) {
	record := legacyPolicyRecord{
		Table: "feature_flags", QueryPolicy: "mysql_page_query_v1", QueryPolicyConfig: `{}`,
		MutationPolicy:       "mysql_single_table_mutation_v1",
		MutationPolicyConfig: `{"auto_fill":{"add":{"creator":{"source":"operator"},"created_at":{"source":"now"},"modifier":{"source":"operator"},"updated_at":{"source":"now"}},"modify":{"modifier":{"source":"operator"},"updated_at":{"source":"now"}}}}`,
		AllowAdd:             true, AllowModify: true,
	}
	plan, problems := planLegacyBackfill(record)
	if len(problems) != 0 {
		t.Fatalf("standard audit Auto Fill was rejected: %v", problems)
	}
	assertField := func(name string, got *string, want string) {
		t.Helper()
		if got == nil || *got != want {
			t.Fatalf("%s: expected %q, got %#v", name, want, got)
		}
	}
	assertField("create operator", plan.mutation.CreateOperatorField, "creator")
	assertField("create time", plan.mutation.CreateTimeField, "created_at")
	assertField("modify operator", plan.mutation.ModifyOperatorField, "modifier")
	assertField("modify time", plan.mutation.ModifyTimeField, "updated_at")
}

func TestLegacyBackfillReportsLiteralArbitraryAndNonMirroredRules(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "literal", config: `{"auto_fill":{"add":{"status":{"source":"literal","value":"READY"}}}}`, want: "literal"},
		{name: "multiple operator targets", config: `{"auto_fill":{"add":{"creator":{"source":"operator"},"owner":{"source":"operator"}}}}`, want: "multiple operator"},
		{name: "modify not mirrored", config: `{"auto_fill":{"modify":{"modifier":{"source":"operator"}}}}`, want: "mirrored"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := legacyPolicyRecord{
				Table: "feature_flags", QueryPolicy: "mysql_page_query_v1", QueryPolicyConfig: `{}`,
				MutationPolicy: "mysql_single_table_mutation_v1", MutationPolicyConfig: test.config,
				AllowAdd: true, AllowModify: true,
			}
			_, problems := planLegacyBackfill(record)
			if len(problems) == 0 || !strings.Contains(strings.ToLower(strings.Join(problems, " ")), test.want) {
				t.Fatalf("expected diagnostic containing %q, got %v", test.want, problems)
			}
		})
	}
}

func TestValidContractionPolicyCodeRejectsTechnologyPrefixes(t *testing.T) {
	for _, code := range []string{
		"mysql_policy_v1", "mariadb_policy_v1", "postgres_policy_v1",
		"postgresql_policy_v1", "sqlite_policy_v1", "oracle_policy_v1",
		"sqlserver_policy_v1", "mongodb_policy_v1", "gorm_policy_v1", "sql_policy_v1",
	} {
		if validContractionPolicyCode(code) {
			t.Fatalf("technology-specific Code passed contraction gate: %s", code)
		}
	}
	if !validContractionPolicyCode("standard_policy_v1") {
		t.Fatal("technology-neutral versioned Code was rejected")
	}
}
