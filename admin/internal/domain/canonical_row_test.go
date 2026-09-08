package domain_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestCanonicalRowFixedLosslessValues(t *testing.T) {
	literal := `{"format":"rcc-admin-mysql-row-v1","schema_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","deleted":false,"fields":[{"name":"nil","type":"varchar(4)","encoding":"sql_null","value":null},{"name":"json","type":"json","encoding":"json","value":"null"},{"name":"text","type":"varchar(4)","encoding":"text","value":"null"},{"name":"empty","type":"varchar(4)","encoding":"text","value":""},{"name":"bytes","type":"varbinary(4)","encoding":"base64","value":"AP+AQQ=="}],"checksum":"de851d5c9c0e005044dd9906b21096e04b29ed8c65aa8161c6c87f630c181cff"}`
	var row domain.CanonicalRow
	if err := json.Unmarshal([]byte(literal), &row); err != nil {
		t.Fatal(err)
	}
	if err := row.Verify(strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	b, err := base64.StdEncoding.DecodeString(*row.Fields[4].Value)
	if err != nil || string(b) != string([]byte{0, 255, 128, 65}) {
		t.Fatal("binary value changed")
	}
	encoded, err := json.Marshal(row)
	if err != nil || string(encoded) != literal {
		t.Fatalf("fixed protocol drift %s %v", encoded, err)
	}
	rebuilt, err := domain.NewCanonicalRow(strings.Repeat("a", 64), false, row.Fields)
	if err != nil || rebuilt.Checksum != row.Checksum {
		t.Fatal("encoder disagrees with independent fixed checksum")
	}
	if row.Verify(strings.Repeat("b", 64)) == nil {
		t.Fatal("wrong schema accepted")
	}
	changed := "changed"
	row.Fields[3].Value = &changed
	if row.Verify(strings.Repeat("a", 64)) == nil {
		t.Fatal("tampered value accepted")
	}
}
func TestCanonicalRowDeletionAndInvalidRepresentations(t *testing.T) {
	digest := strings.Repeat("a", 64)
	deleted, err := domain.NewCanonicalRow(digest, true, nil)
	if err != nil || !deleted.Deleted || len(deleted.Fields) != 0 || deleted.Verify(digest) != nil {
		t.Fatal("invalid tombstone")
	}
	if _, err := domain.NewCanonicalRow(digest, false, nil); err == nil {
		t.Fatal("empty row confused with deletion")
	}
	for _, test := range []struct{ encoding, value string }{{"text", string([]byte{255})}, {"json", "{invalid}"}, {"base64", "AB=="}, {"sql_null", "null"}, {"unknown", "value"}} {
		if _, err := domain.NewCanonicalRow(digest, false, []domain.CanonicalField{{Name: "value", Type: "varchar(10)", Encoding: test.encoding, Value: &test.value}}); err == nil {
			t.Fatalf("invalid representation accepted: %q", test.encoding)
		}
	}
}
