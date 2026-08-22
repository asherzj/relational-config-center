package mysqlstore

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/managedtable"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestEscapeLike(t *testing.T) {
	if got, want := escapeLike("a!b%c_d"), "a!!b!%c!_d"; got != want {
		t.Fatalf("escapeLike() = %q, want %q", got, want)
	}
}

func TestCompiledFilterBindsValuesAndQuotesPolicyColumn(t *testing.T) {
	policy := compilerTestPolicy()
	expression, err := compileFilter(policy, managedtable.Filter{
		Field:    "key",
		Operator: managedtable.OperatorEqual,
		Value:    `x" OR 1=1; DROP TABLE configs`,
	})
	if err != nil {
		t.Fatalf("compileFilter() error = %v", err)
	}
	statement := dryRunDB(t).
		Table(policy.Table).
		Where(expression).
		Find(&[]map[string]any{}).
		Statement
	query := statement.SQL.String()
	if !strings.Contains(query, "`configs`.`config_key` = ?") {
		t.Fatalf("compiled SQL does not contain quoted policy column: %s", query)
	}
	if strings.Contains(query, "DROP TABLE") {
		t.Fatalf("compiled SQL contains client value: %s", query)
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != `x" OR 1=1; DROP TABLE configs` {
		t.Fatalf("bound vars = %#v", statement.Vars)
	}
}

func TestCompiledContainsEscapesWildcards(t *testing.T) {
	policy := compilerTestPolicy()
	expression, err := compileFilter(policy, managedtable.Filter{
		Field:    "key",
		Operator: managedtable.OperatorContains,
		Value:    "discount_%!",
	})
	if err != nil {
		t.Fatalf("compileFilter() error = %v", err)
	}
	statement := dryRunDB(t).
		Table(policy.Table).
		Where(expression).
		Find(&[]map[string]any{}).
		Statement
	if !strings.Contains(statement.SQL.String(), "LIKE ? ESCAPE '!'") {
		t.Fatalf("compiled SQL = %s", statement.SQL.String())
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != "%discount!_!%!!%" {
		t.Fatalf("bound vars = %#v", statement.Vars)
	}
}

func TestCompiledFilterPreservesNestedAndOrTree(t *testing.T) {
	policy := compilerTestPolicy()
	policy.Fields["status"] = managedtable.FieldPolicy{
		Column:   "status",
		Type:     managedtable.TypeString,
		Readable: true,
	}
	expression, err := compileFilter(policy, managedtable.Filter{
		Logic: managedtable.LogicAnd,
		Items: []managedtable.Filter{
			{Field: "key", Operator: managedtable.OperatorContains, Value: "checkout"},
			{
				Logic: managedtable.LogicOr,
				Items: []managedtable.Filter{
					{Field: "status", Operator: managedtable.OperatorEqual, Value: "draft"},
					{Field: "status", Operator: managedtable.OperatorEqual, Value: "published"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("compileFilter() error = %v", err)
	}
	statement := dryRunDB(t).
		Table(policy.Table).
		Where(expression).
		Find(&[]map[string]any{}).
		Statement
	query := statement.SQL.String()
	if !strings.Contains(query, "AND (`configs`.`status` = ? OR `configs`.`status` = ?)") {
		t.Fatalf("nested filter SQL = %s", query)
	}
	if len(statement.Vars) != 3 {
		t.Fatalf("nested filter vars = %#v", statement.Vars)
	}
}

func TestSelectAndSortUseOnlyPolicyMappings(t *testing.T) {
	policy := compilerTestPolicy()
	statement := dryRunDB(t).
		Table(policy.Table).
		Clauses(
			readableSelect(policy),
			orderBy(policy, []managedtable.Sort{{Field: "key", Direction: managedtable.DirectionDescending}}),
		).
		Limit(20).
		Find(&[]map[string]any{}).
		Statement
	query := statement.SQL.String()
	for _, fragment := range []string{
		"`configs`.`config_key` AS `key`",
		"ORDER BY `configs`.`config_key` DESC",
		"LIMIT ?",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("compiled SQL %q does not contain %q", query, fragment)
		}
	}
}

func TestGenericMapMutationsBuildScopedSQL(t *testing.T) {
	db := dryRunDB(t)
	created := db.Table("configs").Create(map[string]any{
		"namespace":    "default",
		"config_key":   "feature.flag",
		"config_value": []byte(`{"enabled":true}`),
	})
	if created.Error != nil || !strings.HasPrefix(created.Statement.SQL.String(), "INSERT INTO `configs`") {
		t.Fatalf("create SQL = %q, error = %v", created.Statement.SQL.String(), created.Error)
	}

	updated := db.Table("configs").
		Where("`id` = ?", uint64(1)).
		Updates(map[string]any{"status": "published"})
	if updated.Error != nil || !strings.HasPrefix(updated.Statement.SQL.String(), "UPDATE `configs`") {
		t.Fatalf("update SQL = %q, error = %v", updated.Statement.SQL.String(), updated.Error)
	}
	if !strings.Contains(updated.Statement.SQL.String(), "WHERE `id` = ?") {
		t.Fatalf("update lacks primary-key condition: %s", updated.Statement.SQL.String())
	}

	deleted := db.Table("configs").
		Where("`id` = ?", uint64(1)).
		Delete(&map[string]any{})
	if deleted.Error != nil || !strings.HasPrefix(deleted.Statement.SQL.String(), "DELETE FROM `configs`") {
		t.Fatalf("delete SQL = %q, error = %v", deleted.Statement.SQL.String(), deleted.Error)
	}
	if !strings.Contains(deleted.Statement.SQL.String(), "WHERE `id` = ?") {
		t.Fatalf("delete lacks primary-key condition: %s", deleted.Statement.SQL.String())
	}
}

func TestNormalizeRowUsesPublicFieldTypes(t *testing.T) {
	policy := managedtable.Policy{Fields: map[string]managedtable.FieldPolicy{
		"id":      {Type: managedtable.TypeUnsigned, Readable: true},
		"name":    {Type: managedtable.TypeString, Readable: true},
		"enabled": {Type: managedtable.TypeBoolean, Readable: true},
		"value":   {Type: managedtable.TypeJSON, Readable: true},
	}}
	row := map[string]any{
		"id":      []byte("42"),
		"name":    []byte("feature.flag"),
		"enabled": []byte("1"),
		"value":   []byte(`{"ratio":0.5}`),
	}
	normalizeRow(policy, row)
	if row["id"] != "42" || row["name"] != "feature.flag" || row["enabled"] != true {
		t.Fatalf("normalized row = %#v", row)
	}
	if !reflect.DeepEqual(row["value"], map[string]any{"ratio": json.Number("0.5")}) {
		t.Fatalf("normalized JSON = %#v", row["value"])
	}
}

func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{
		DSN:                       "user:pass@tcp(127.0.0.1:3306)/rcc",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open dry-run GORM: %v", err)
	}
	return db
}

func compilerTestPolicy() managedtable.Policy {
	return managedtable.Policy{
		Resource:   "configs",
		Table:      "configs",
		PrimaryKey: "id",
		Fields: map[string]managedtable.FieldPolicy{
			"id": {
				Column:        "id",
				Type:          managedtable.TypeUnsigned,
				Readable:      true,
				Sortable:      true,
				AutoIncrement: true,
			},
			"key": {
				Column:   "config_key",
				Type:     managedtable.TypeString,
				Readable: true,
				Sortable: true,
			},
		},
	}
}
