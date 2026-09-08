package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const CanonicalRowFormat = "rcc-admin-mysql-row-v1"

var ErrCanonicalRow = errors.New("invalid canonical row")

// Fields are in schema ordinal order. Text uses UTF-8, bytes use canonical
// base64, JSON retains its database text, and SQL NULL has no value.
type CanonicalField struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Encoding string  `json:"encoding"`
	Value    *string `json:"value"`
}
type CanonicalRow struct {
	Format       string           `json:"format"`
	SchemaDigest string           `json:"schema_digest"`
	Deleted      bool             `json:"deleted"`
	Fields       []CanonicalField `json:"fields"`
	Checksum     string           `json:"checksum"`
}

func NewCanonicalRow(schemaDigest string, deleted bool, fields []CanonicalField) (CanonicalRow, error) {
	row := CanonicalRow{Format: CanonicalRowFormat, SchemaDigest: schemaDigest, Deleted: deleted, Fields: fields}
	if row.Fields == nil {
		row.Fields = []CanonicalField{}
	}
	if err := row.validateContent(); err != nil {
		return CanonicalRow{}, err
	}
	row.Checksum = row.checksum()
	return row, nil
}
func (row CanonicalRow) Verify(schemaDigest string) error {
	if row.SchemaDigest != schemaDigest || row.validateContent() != nil || row.Checksum != row.checksum() {
		return ErrCanonicalRow
	}
	return nil
}
func (row CanonicalRow) checksum() string {
	row.Checksum = ""
	encoded, _ := json.Marshal(row)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
func (row CanonicalRow) validateContent() error {
	digest, err := hex.DecodeString(row.SchemaDigest)
	if row.Format != CanonicalRowFormat || err != nil || len(digest) != 32 || hex.EncodeToString(digest) != row.SchemaDigest || row.Deleted && len(row.Fields) != 0 || !row.Deleted && len(row.Fields) == 0 {
		return ErrCanonicalRow
	}
	names := map[string]bool{}
	for _, field := range row.Fields {
		if field.Name == "" || names[field.Name] || field.Type == "" || !utf8.ValidString(field.Name) || !utf8.ValidString(field.Type) {
			return ErrCanonicalRow
		}
		names[field.Name] = true
		if field.Encoding == "sql_null" {
			if field.Value != nil {
				return ErrCanonicalRow
			}
			continue
		}
		if field.Value == nil {
			return ErrCanonicalRow
		}
		switch field.Encoding {
		case "text":
			if !utf8.ValidString(*field.Value) {
				return ErrCanonicalRow
			}
		case "json":
			if !utf8.ValidString(*field.Value) || !json.Valid([]byte(*field.Value)) {
				return ErrCanonicalRow
			}
		case "base64":
			b, err := base64.StdEncoding.DecodeString(*field.Value)
			if err != nil || base64.StdEncoding.EncodeToString(b) != *field.Value {
				return ErrCanonicalRow
			}
		default:
			return ErrCanonicalRow
		}
	}
	return nil
}

// RecordID is the public identity of the actual stored row, including an empty
// string. TIMESTAMP identities use the same UTC representation as query inputs.
func (row CanonicalRow) RecordID() (string, error) {
	if row.Deleted {
		return "", ErrCanonicalRow
	}
	for _, field := range row.Fields {
		if field.Name == "id" && field.Encoding == "text" && field.Value != nil {
			value := *field.Value
			if strings.HasPrefix(strings.ToLower(field.Type), "timestamp") {
				value = strings.Replace(value, " ", "T", 1) + "Z"
			}
			return value, nil
		}
	}
	return "", ErrCanonicalRow
}
