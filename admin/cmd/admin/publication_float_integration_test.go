//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestPublicationFloatValuesAndIdentityRoundTrip(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	for _, ddl := range []string{
		"CREATE TABLE float_publication_values (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,v FLOAT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE float_publication_ids (id FLOAT PRIMARY KEY,v FLOAT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE double_publication_ids (id DOUBLE PRIMARY KEY,v FLOAT NOT NULL) ENGINE=InnoDB",
	} {
		deliveryExec(t, db, ddl)
	}
	for _, table := range []string{"float_publication_values", "float_publication_ids", "double_publication_ids"} {
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	}
	expected := "1.2345678806304932"
	t.Run("ordinary FLOAT final value is lossless", func(t *testing.T) {
		command := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "float_publication_values", "", `{"content":{"v":"1.2345678806304932"}}`))
		for _, field := range command.Final.Fields {
			if field.Name == "v" && (field.Value == nil || *field.Value != expected) {
				t.Fatalf("FLOAT value lost bits: %+v", field)
			}
		}
		row, version := recordVersionRow(t, app, "float_publication_values", command.ID)
		if *row["v"] != expected || version != "1" {
			t.Fatalf("query lost FLOAT: %v %s", row, version)
		}
		var equal bool
		if err := db.QueryRow("SELECT CAST(? AS FLOAT)=v FROM float_publication_values WHERE id=?", expected, command.ID).Scan(&equal); err != nil || !equal {
			t.Fatalf("value cannot be restored: %v %v", equal, err)
		}
	})
	t.Run("adjacent FLOAT ids remain distinct and returned id is reusable", func(t *testing.T) {
		for _, id := range []string{"1.2345670461654663", expected} {
			body, _ := json.Marshal(map[string]any{"content": map[string]string{"id": id, "v": expected}})
			command := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "float_publication_ids", "", string(body)))
			if command.ID != id || command.RecordVersion != "1" {
				t.Fatalf("actual FLOAT identity: %+v", command)
			}
			row, version := recordVersionRow(t, app, "float_publication_ids", command.ID)
			if *row["id"] != command.ID || version != "1" {
				t.Fatalf("returned id cannot query same row: %v %s", row, version)
			}
			changed := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "MODIFY", "float_publication_ids", command.ID, `{"content":{"v":"2.5"},"expected_version":"1"}`))
			if changed.RecordVersion != "2" {
				t.Fatal(changed)
			}
		}
		var keys int
		if err := db.QueryRow("SELECT COUNT(*) FROM rcc_record_versions WHERE table_name='float_publication_ids' AND LENGTH(record_key)=32").Scan(&keys); err != nil || keys != 2 {
			t.Fatalf("distinct FLOAT ids collided: %d %v", keys, err)
		}
	})
	for _, table := range []string{"float_publication_ids", "double_publication_ids"} {
		t.Run(table+" signed zero shares the tombstone", func(t *testing.T) {
			added := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", table, "", `{"content":{"id":"-0","v":"2.5"}}`))
			if added.RecordVersion != "1" {
				t.Fatal(added)
			}
			deleted := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "DELETE", table, "0", `{"expected_version":"1"}`))
			if deleted.RecordVersion != "2" {
				t.Fatal(deleted)
			}
			rebuilt := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", table, "", `{"content":{"id":"0","v":"3.5"},"expected_version":"2"}`))
			if rebuilt.RecordVersion != "3" {
				t.Fatal(rebuilt)
			}
		})
	}
	t.Run("decimal input is converted to stored FLOAT before lookup", func(t *testing.T) {
		added := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "float_publication_ids", "", `{"content":{"id":"0.1","v":"0.1"}}`))
		if added.ID != "0.10000000149011612" {
			t.Fatal(added.ID)
		}
		row, version := recordVersionRow(t, app, "float_publication_ids", "0.1")
		if *row["id"] != added.ID || version != "1" {
			t.Fatalf("stored FLOAT lookup: %v %s", row, version)
		}
	})
}

func TestPublicationFloatIdentityMaintenanceKeepsOldVersions(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	deliveryExec(t, db, "CREATE TABLE float_legacy_ids (id FLOAT PRIMARY KEY,v FLOAT NOT NULL) ENGINE=InnoDB")
	deliveryExec(t, db, "INSERT INTO float_legacy_ids VALUES(1.2345670461654663,1),(1.2345678806304932,2)")
	// Old code produced one short-text key for these two distinct primary keys.
	deliveryExec(t, db, "INSERT INTO rcc_record_versions SELECT 'float_legacy_ids',UNHEX(SHA2(WEIGHT_STRING(CAST(id AS CHAR CHARACTER SET ascii)),256)),5 FROM float_legacy_ids LIMIT 1")
	// Existing ADR-0021 maintenance protocol, performed while all old requests
	// are drained and submitted/approved orders have been cancelled by old Admin.
	deliveryExec(t, db, "INSERT INTO rcc_record_versions SELECT 'float_legacy_ids',X'',MAX(lock_version)+1 FROM rcc_record_versions WHERE table_name='float_legacy_ids'")
	enableMutationPolicy(t, app, "float_legacy_ids", mutationPolicyFixture{AllowModify: true})
	for _, id := range []string{"1.2345670461654663", "1.2345678806304932"} {
		_, version := recordVersionRow(t, app, "float_legacy_ids", id)
		if version != "6" {
			t.Fatalf("legacy token reset: %s", version)
		}
	}
	assertIntegrationErrorCode(t, publicationFixtureRequest(t, app, "MODIFY", "float_legacy_ids", "1.2345670461654663", `{"content":{"v":"3"},"expected_version":"5"}`), 409, "record_version_conflict")
	command := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "MODIFY", "float_legacy_ids", "1.2345670461654663", `{"content":{"v":"3"},"expected_version":"6"}`))
	if command.RecordVersion != "7" {
		t.Fatal(command)
	}
	_, second := recordVersionRow(t, app, "float_legacy_ids", "1.2345678806304932")
	if second != "6" {
		t.Fatalf("unrelated FLOAT identity advanced: %s", second)
	}
	var old int
	if err := db.QueryRow("SELECT lock_version FROM rcc_record_versions WHERE table_name='float_legacy_ids' AND record_key=UNHEX(SHA2(WEIGHT_STRING(CAST(CAST(1.2345678806304932 AS FLOAT) AS CHAR CHARACTER SET ascii)),256))").Scan(&old); err != nil || old != 5 {
		t.Fatal(fmt.Sprintf("old version removed or changed: %d %v", old, err))
	}
	deliveryExec(t, db, "CREATE TABLE double_legacy_zero (id DOUBLE PRIMARY KEY,v FLOAT NOT NULL) ENGINE=InnoDB")
	deliveryExec(t, db, "INSERT INTO double_legacy_zero VALUES(CAST('-0' AS DOUBLE),1)")
	deliveryExec(t, db, "INSERT INTO rcc_record_versions SELECT 'double_legacy_zero',UNHEX(SHA2(WEIGHT_STRING(CAST(id AS CHAR CHARACTER SET ascii)),256)),5 FROM double_legacy_zero")
	deliveryExec(t, db, "INSERT INTO rcc_record_versions SELECT 'double_legacy_zero',X'',MAX(lock_version)+1 FROM rcc_record_versions WHERE table_name='double_legacy_zero'")
	enableMutationPolicy(t, app, "double_legacy_zero", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	_, zeroVersion := recordVersionRow(t, app, "double_legacy_zero", "0")
	if zeroVersion != "6" {
		t.Fatalf("DOUBLE negative-zero legacy version reset: %s", zeroVersion)
	}
	assertIntegrationErrorCode(t, publicationFixtureRequest(t, app, "DELETE", "double_legacy_zero", "0", `{"expected_version":"5"}`), 409, "record_version_conflict")
	deleted := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "DELETE", "double_legacy_zero", "0", `{"expected_version":"6"}`))
	if deleted.RecordVersion != "7" {
		t.Fatal(deleted)
	}
	rebuilt := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "double_legacy_zero", "", `{"content":{"id":"0","v":"1"},"expected_version":"7"}`))
	if rebuilt.RecordVersion != "8" {
		t.Fatal(rebuilt)
	}
	if err := db.QueryRow("SELECT lock_version FROM rcc_record_versions WHERE table_name='double_legacy_zero' AND record_key=UNHEX(SHA2(WEIGHT_STRING(CAST('-0' AS CHAR CHARACTER SET ascii)),256))").Scan(&old); err != nil || old != 5 {
		t.Fatalf("DOUBLE old negative-zero version changed: %d %v", old, err)
	}
}
