package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var schemaAutoIncrement = regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)
var schemaTableName = regexp.MustCompile(`^rcc_[a-z][a-z0-9_]*$`)

// Recovery checks the actual definitions of surviving tables before replaying
// the restartable baseline. IF NOT EXISTS alone does not prove compatibility.
func checkControlSchema(ctx context.Context, db schemaQuerier, version int64, allowMissing bool) error {
	raw, err := controlSchemaFiles.ReadFile(fmt.Sprintf("migrations/%05d_schema.json", version))
	if err != nil {
		return errors.New("migration_invalid: baseline manifest unavailable")
	}
	var expected map[string]string
	if json.Unmarshal(raw, &expected) != nil || len(expected) == 0 {
		return errors.New("migration_invalid: baseline manifest invalid")
	}
	names := make([]string, 0, len(expected))
	for name := range expected {
		if !schemaTableName.MatchString(name) {
			return errors.New("migration_invalid: invalid control table name in manifest")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var present int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, name).Scan(&present); err != nil {
			return errors.New("schema_unavailable: cannot inspect control structure")
		}
		if present == 0 && allowMissing {
			continue
		}
		if present == 0 {
			return fmt.Errorf("schema_mismatch: missing control table %s", name)
		}
		var table, actual string
		if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE `"+name+"`").Scan(&table, &actual); err != nil {
			return fmt.Errorf("schema_unavailable: cannot inspect control table %s", name)
		}
		if strings.TrimSpace(schemaAutoIncrement.ReplaceAllString(actual, "")) != strings.TrimSpace(expected[name]) {
			return fmt.Errorf("schema_mismatch: incompatible control table %s; repair before recovery", name)
		}
	}
	if !allowMissing {
		var rows, sentinel int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(id=1),0) FROM rcc_auth_control_lock`).Scan(&rows, &sentinel); err != nil || rows != 1 || sentinel != 1 {
			return errors.New("schema_mismatch: authentication control lock must retain its single required row")
		}
	}
	return nil
}
