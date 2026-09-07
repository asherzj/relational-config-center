package application

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrRecordVersionRequired     = errors.New("record version is required")
	ErrRecordVersionInvalid      = errors.New("record version must be a canonical unsigned decimal string")
	ErrRecordVersionConflict     = errors.New("record version conflict")
	ErrOperatorFieldIncompatible = errors.New("operator field cannot store a complete Account ID")
	ErrMutationNotAllowed        = errors.New("mutation operation is not allowed")
	ErrInvalidMutation           = errors.New("invalid mutation content")
	ErrMissingRequiredField      = errors.New("required mutation field is missing")
	ErrDuplicateKey              = errors.New("duplicate key")
	ErrMutationRowNotFound       = errors.New("mutation row not found")
	ErrMutationUnavailable       = errors.New("mutation unavailable")
	ErrMutationTimeout           = errors.New("mutation timeout")
)

func currentTimeValue(column domain.Column, now time.Time) (domain.JSONString, error) {
	now = now.Truncate(time.Microsecond)
	switch column.Type {
	case domain.ColumnTypeDate:
		return domain.JSONString(now.Format("2006-01-02")), nil
	case domain.ColumnTypeTime:
		return domain.JSONString(formatAutoFillTime(now, "15:04:05", "")), nil
	case domain.ColumnTypeDateTime:
		return domain.JSONString(formatAutoFillTime(now, "2006-01-02 15:04:05", "")), nil
	case domain.ColumnTypeTimestamp:
		return domain.JSONString(formatAutoFillTime(now, "2006-01-02T15:04:05", "Z")), nil
	default:
		return "", ErrInvalidMutation
	}
}

func formatAutoFillTime(value time.Time, layout, suffix string) string {
	formatted := value.Format(layout)
	if microseconds := value.Nanosecond() / 1000; microseconds != 0 {
		formatted += "." + strings.TrimRight(fmt.Sprintf("%06d", microseconds), "0")
	}
	return formatted + suffix
}

func mutationValues(schema domain.TableSchema, content domain.MutationContent, allowID bool) ([]domain.MutationValue, error) {
	values := make([]domain.MutationValue, 0, len(content))
	for field, value := range content {
		if field == "id" && !allowID {
			return nil, ErrInvalidMutation
		}
		column, found := schema.Column(field)
		if !found || !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
			return nil, ErrInvalidMutation
		}
		if value == nil {
			if !column.Nullable {
				return nil, ErrInvalidMutation
			}
			values = append(values, domain.MutationValue{Column: column})
			continue
		}
		parsed, err := domain.ParseColumnValue(column, *value)
		if err != nil {
			return nil, ErrInvalidMutation
		}
		values = append(values, domain.MutationValue{Column: column, Value: parsed})
	}
	return values, nil
}

// ValidateRecordVersion rejects a missing baseline rather than adopting the latest value.
func ValidateRecordVersion(version string) error {
	if version == "" {
		return ErrRecordVersionRequired
	}
	value, err := strconv.ParseUint(version, 10, 64)
	if err != nil || strconv.FormatUint(value, 10) != version {
		return ErrRecordVersionInvalid
	}
	return nil
}

func validateOperatorField(schema domain.TableSchema, field string) error {
	column, found := schema.Column(field)
	if !found || column.Type != domain.ColumnTypeString || column.TextCapacity < 36 {
		return ErrOperatorFieldIncompatible
	}
	return nil
}
