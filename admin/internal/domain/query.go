package domain

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"time"
)

// ColumnType is the lossless JSON-string representation exposed for a live
// Managed Table column.
type ColumnType string

const (
	ColumnTypeUInt64      ColumnType = "uint64"
	ColumnTypeInt64       ColumnType = "int64"
	ColumnTypeDecimal     ColumnType = "decimal"
	ColumnTypeFloat64     ColumnType = "float64"
	ColumnTypeString      ColumnType = "string"
	ColumnTypeBoolean     ColumnType = "boolean"
	ColumnTypeDate        ColumnType = "date"
	ColumnTypeTime        ColumnType = "time"
	ColumnTypeDateTime    ColumnType = "datetime"
	ColumnTypeTimestamp   ColumnType = "timestamp"
	ColumnTypeJSON        ColumnType = "json"
	ColumnTypeUnsupported ColumnType = "unsupported"
)

type Column struct {
	// TextCapacity is the live character capacity of unrestricted text columns.
	// Zero means the column cannot hold arbitrary Operator identifiers (e.g. ENUM).
	TextCapacity  uint64
	Name          string
	Type          ColumnType
	Nullable      bool
	Generated     bool
	AutoIncrement bool
	HasDefault    bool
}

func (column Column) Writable() bool {
	return !column.Generated
}

func (column Column) RequiredForInsert() bool {
	return !column.Nullable && !column.Generated && !column.AutoIncrement && !column.HasDefault
}

type TableSchema struct {
	Name                  string
	Columns               []Column
	Compatible            bool
	IncompatibilityReason *IncompatibilityReason
}

func (schema TableSchema) Column(name string) (Column, bool) {
	for _, column := range schema.Columns {
		if column.Name == name {
			return column, true
		}
	}
	return Column{}, false
}

type JSONString string

var (
	decimalValuePattern   = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$`)
	timeValuePattern      = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]{1,6})?$`)
	timestampValuePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,6})?Z$`)
)

// ParseColumnValue validates and converts one JSON String according to live
// Schema metadata. Query Policy validation and the MySQL adapter share this
// parser so accepted values cannot drift from bound values.
func ParseColumnValue(column Column, value JSONString) (any, error) {
	text := string(value)
	switch column.Type {
	case ColumnTypeUInt64:
		return strconv.ParseUint(text, 10, 64)
	case ColumnTypeInt64:
		return strconv.ParseInt(text, 10, 64)
	case ColumnTypeDecimal:
		if !decimalValuePattern.MatchString(text) {
			return nil, errors.New("invalid decimal")
		}
		return text, nil
	case ColumnTypeFloat64:
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
			return nil, errors.New("invalid float")
		}
		return parsed, nil
	case ColumnTypeString:
		return text, nil
	case ColumnTypeBoolean:
		if text != "0" && text != "1" {
			return nil, errors.New("invalid boolean")
		}
		return strconv.ParseInt(text, 10, 8)
	case ColumnTypeDate:
		return time.ParseInLocation("2006-01-02", text, time.UTC)
	case ColumnTypeTime:
		if !timeValuePattern.MatchString(text) {
			return nil, errors.New("invalid time")
		}
		return text, nil
	case ColumnTypeDateTime:
		return time.ParseInLocation("2006-01-02 15:04:05.999999", text, time.UTC)
	case ColumnTypeTimestamp:
		if !timestampValuePattern.MatchString(text) {
			return nil, errors.New("timestamp must be RFC 3339 UTC with microsecond precision")
		}
		return time.Parse(time.RFC3339Nano, text)
	case ColumnTypeJSON:
		if !json.Valid([]byte(text)) {
			return nil, errors.New("invalid JSON")
		}
		return text, nil
	default:
		return nil, errors.New("unsupported column type")
	}
}

func (column Column) SupportsContains() bool {
	return column.Type == ColumnTypeString
}

func (column Column) SupportsRange() bool {
	return column.Type != ColumnTypeBoolean && column.Type != ColumnTypeJSON && column.Type != ColumnTypeUnsupported
}

type QueryOperator string

const (
	QueryOperatorExact       QueryOperator = "exact"
	QueryOperatorContains    QueryOperator = "contains"
	QueryOperatorOpenRange   QueryOperator = "open_range"
	QueryOperatorClosedRange QueryOperator = "closed_range"
	QueryOperatorIn          QueryOperator = "in"
	QueryOperatorNotIn       QueryOperator = "not_in"
	QueryOperatorIsNull      QueryOperator = "is_null"
	QueryOperatorIsNotNull   QueryOperator = "is_not_null"
)

type QueryCondition struct {
	Field         string
	Operator      QueryOperator
	Value         *JSONString
	ValuePresent  bool
	From          *JSONString
	FromPresent   bool
	To            *JSONString
	ToPresent     bool
	Values        []*JSONString
	ValuesPresent bool
}

type QueryOrder struct {
	Field     string
	Direction string
}

type QuerySpec struct {
	Conditions []QueryCondition
	Order      *QueryOrder
	PageNumber int
	PageSize   int
}

// PageQuery is a fully validated storage-neutral execution request. The MySQL
// adapter owns compilation, transaction handling, and rollback behind its
// application interface.
type PageQuery struct {
	TableName  string
	Columns    []Column
	Conditions []QueryCondition
	Order      QueryOrder
	PageNumber int
	PageSize   int
	Offset     int
}

type Row map[string]*JSONString

type Page struct {
	PageNumber int
	PageSize   int
	TotalCount int64
	TotalPages int64
}

type QueryResult struct {
	Columns []Column
	Rows    []Row
	Page    Page
}
