package application

import (
	"errors"
	"reflect"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestPolicySnapshotResolverAllowsAssignedDeprecatedDefinitions(t *testing.T) {
	reader := validQuerySnapshotSession()
	reader.queryPolicy.Status = domain.PolicyStatusDeprecated
	reader.mutationPolicy.Status = domain.PolicyStatusDeprecated
	resolver := newPolicySnapshotResolver(NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())

	if _, err := resolver.resolve(t.Context(), reader, "managed_items", queryPolicySnapshot); err != nil {
		t.Fatalf("assigned Deprecated definitions must resolve: %v", err)
	}
}

func TestPolicySnapshotResolverFailsClosedBeforeExecution(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*memoryQuerySnapshotSession)
		want      error
		wantCalls []string
	}{
		{
			name:      "disabled assignment still reads complete definitions",
			edit:      func(reader *memoryQuerySnapshotSession) { reader.tablePolicy.Enabled = false },
			want:      ErrTablePolicyDisabled,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "Draft Query Policy",
			edit:      func(reader *memoryQuerySnapshotSession) { reader.queryPolicy.Status = domain.PolicyStatusDraft },
			want:      ErrInvalidPolicySnapshot,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "Draft Mutation Policy",
			edit:      func(reader *memoryQuerySnapshotSession) { reader.mutationPolicy.Status = domain.PolicyStatusDraft },
			want:      ErrInvalidPolicySnapshot,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "unknown Query Type",
			edit:      func(reader *memoryQuerySnapshotSession) { reader.queryPolicy.TypeCode = "unknown" },
			want:      ErrUnknownQueryPolicyType,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "unknown Mutation Type",
			edit:      func(reader *memoryQuerySnapshotSession) { reader.mutationPolicy.TypeCode = "unknown" },
			want:      ErrUnknownMutationPolicyType,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "Policy cannot relax code-owned page maximum",
			edit:      func(reader *memoryQuerySnapshotSession) { reader.queryPolicy.MaxPageSize = 500 },
			want:      ErrInvalidQueryPolicyRules,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1"},
		},
		{
			name:      "incompatible live default order",
			edit:      func(reader *memoryQuerySnapshotSession) { reader.queryPolicy.DefaultOrderField = "missing" },
			want:      ErrInvalidQueryPolicyRules,
			wantCalls: []string{"table:managed_items", "query:standard_page_query_v1", "mutation:standard_mutation_v1", "schema:managed_items"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := validQuerySnapshotSession()
			test.edit(reader)
			resolver := newPolicySnapshotResolver(NewQueryPolicyTypeRegistry(), NewMutationPolicyTypeRegistry())
			if _, err := resolver.resolve(t.Context(), reader, "managed_items", queryPolicySnapshot); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if !reflect.DeepEqual(reader.calls, test.wantCalls) {
				t.Fatalf("unexpected reads before fail-closed result: %#v", reader.calls)
			}
		})
	}
}

var _ PolicySnapshotReader = (*memoryQuerySnapshotSession)(nil)
