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
var schemaTableComment = regexp.MustCompile(` COMMENT='(?:[^'\\]|\\.|'')*'$`)
var schemaTableName = regexp.MustCompile(`^rcc_[a-z][a-z0-9_]*$`)

func comparableControlDefinition(definition string) string {
	// Historical Policy contraction did not set the descriptive table comment.
	// Neither that comment nor a retained AUTO_INCREMENT counter changes schema.
	definition = strings.TrimSpace(definition)
	if footer := strings.LastIndex(definition, "\n) ENGINE="); footer >= 0 {
		options := schemaTableComment.ReplaceAllString(definition[footer:], "")
		return definition[:footer] + schemaAutoIncrement.ReplaceAllString(options, "")
	}
	return definition
}

func controlSchemaManifest(version int64) (map[string]string, error) {
	raw, err := controlSchemaFiles.ReadFile(fmt.Sprintf("migrations/%05d_schema.json", version))
	if err != nil {
		return nil, errors.New("migration_invalid: baseline manifest unavailable")
	}
	var expected map[string]string
	if json.Unmarshal(raw, &expected) != nil || len(expected) == 0 {
		return nil, errors.New("migration_invalid: baseline manifest invalid")
	}
	return expected, nil
}

// Restartable migrations may leave a table at its previous or target definition.
// Existing tables must remain present; only newly introduced tables may be absent.
func checkControlSchema(ctx context.Context, db schemaQuerier, version int64, recovering bool) error {
	expected, err := controlSchemaManifest(version)
	if err != nil {
		return err
	}
	var previous map[string]string
	if recovering {
		versions, err := controlSchemaVersions()
		if err != nil {
			return err
		}
		for index, candidate := range versions {
			if candidate == version && index > 0 {
				previous, err = controlSchemaManifest(versions[index-1])
				if err != nil {
					return err
				}
			}
		}
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
		if present == 0 && recovering && previous[name] == "" {
			continue
		}
		if present == 0 {
			return fmt.Errorf("schema_mismatch: missing control table %s", name)
		}
		var table, actual string
		if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE `"+name+"`").Scan(&table, &actual); err != nil {
			return fmt.Errorf("schema_unavailable: cannot inspect control table %s", name)
		}
		normalized := comparableControlDefinition(actual)
		if normalized != comparableControlDefinition(expected[name]) && (!recovering || normalized != comparableControlDefinition(previous[name])) {
			return fmt.Errorf("schema_mismatch: incompatible control table %s; repair before recovery", name)
		}
	}
	if !recovering || previous != nil {
		var rows, sentinel int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(id=1),0) FROM rcc_auth_control_lock`).Scan(&rows, &sentinel); err != nil || rows != 1 || sentinel != 1 {
			return errors.New("schema_mismatch: authentication control lock must retain its single required row")
		}
		// The published account schema predates SQL CHECK constraints for these
		// values. Preserve the existing read-only account readiness invariant.
		var invalidRoles int
		invalid := "roles < 1 OR roles > 31 OR role_version < 1"
		// Only an unfinished v9 may retain old grants while explicitly recovering.
		// Historical baseline/v6-v8 validation must still accept their valid data.
		if version >= 9 && !(version == 9 && recovering) {
			invalid += " OR roles & 4 <> 0"
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rcc_accounts WHERE `+invalid).Scan(&invalidRoles); err != nil || invalidRoles != 0 {
			return errors.New("schema_mismatch: account roles and role versions must remain valid")
		}
	}
	return nil
}
