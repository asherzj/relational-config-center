package domain

import (
	"reflect"
	"testing"
)

// Public types enforce the current-grant versus immutable-history boundary.
func TestCurrentAndHistoricalAccountRoleContracts(t *testing.T) {
	for _, sample := range []struct {
		name  string
		value AccountRoles
	}{{"VIEWER", 1}, {"EDITOR", 2}, {"PUBLISHER", 8}, {"ADMIN", 16}} {
		roles, err := ParseAccountRoles([]string{sample.name})
		if err != nil || roles != sample.value || !roles.Allows(RoleViewer) || !reflect.DeepEqual(roles.Names(), []string{sample.name}) {
			t.Fatalf("stable current grant %s: %d %v", sample.name, roles, err)
		}
	}
	for _, old := range []AccountRoles{0, 4, 12, 20, 31, 32} {
		if old.Valid() || old.Allows(RoleViewer) || old.Allows(RoleEditor) || old.Allows(RolePublisher) || old.Allows(RoleAdmin) || len(old.Names()) != 0 {
			t.Fatalf("invalid current roles %d translated into capability", old)
		}
	}
	if _, err := ParseAccountRoles([]string{"APPROVER"}); err != ErrInvalidRoles {
		t.Fatal("retired grant parsed as current input")
	}
	if _, ok := any(HistoricalAccountRoles(31)).(interface{ Allows(AccountRoles) bool }); ok {
		t.Fatal("historical decoder exposes current authorization")
	}
	if !reflect.DeepEqual(HistoricalAccountRoles(31).Names(), []string{"VIEWER", "EDITOR", "APPROVER", "PUBLISHER", "ADMIN"}) || !reflect.DeepEqual(HistoricalAccountRoles(4).Names(), []string{"APPROVER"}) {
		t.Fatal("historical grants lost their original meaning")
	}
}
