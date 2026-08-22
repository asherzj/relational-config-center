package managedtable

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func normalizeValue(path string, field FieldPolicy, value any) (any, error) {
	if value == nil {
		if !field.Nullable {
			return nil, Invalid(path, "null is not allowed")
		}
		return nil, nil
	}

	var normalized any
	var err error
	switch field.Type {
	case TypeString:
		normalized, err = normalizeString(value)
	case TypeInteger:
		normalized, err = normalizeInteger(value)
	case TypeUnsigned:
		normalized, err = normalizeUnsigned(value)
	case TypeBoolean:
		normalized, err = normalizeBoolean(value)
	case TypeJSON:
		normalized, err = normalizeJSON(value)
	case TypeTime:
		normalized, err = normalizeTime(value)
	default:
		err = fmt.Errorf("unsupported field type %q", field.Type)
	}
	if err != nil {
		return nil, Invalid(path, err.Error())
	}

	if text, ok := normalized.(string); ok {
		if field.MinLength > 0 && len([]rune(text)) < field.MinLength {
			return nil, Invalid(path, fmt.Sprintf("must contain at least %d characters", field.MinLength))
		}
		if field.MaxLength > 0 && len([]rune(text)) > field.MaxLength {
			return nil, Invalid(path, fmt.Sprintf("must contain at most %d characters", field.MaxLength))
		}
		if len(field.AllowedValues) > 0 && !containsString(field.AllowedValues, text) {
			return nil, Invalid(path, fmt.Sprintf("must be one of: %s", strings.Join(field.AllowedValues, ", ")))
		}
	}
	return normalized, nil
}

func normalizePathValue(path, raw string, field FieldPolicy) (any, error) {
	switch field.Type {
	case TypeString:
		return normalizeValue(path, field, raw)
	case TypeInteger:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, Invalid(path, "must be a base-10 integer")
		}
		return value, nil
	case TypeUnsigned:
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, Invalid(path, "must be a base-10 unsigned integer")
		}
		return value, nil
	default:
		return nil, Invalid(path, "primary keys must use a string or integer type")
	}
}

func normalizeString(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("must be a string")
	}
	return text, nil
}

func normalizeInteger(value any) (int64, error) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Int64()
		if err != nil {
			return 0, fmt.Errorf("must be a 64-bit integer")
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseInt(number, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("must be a 64-bit integer")
		}
		return parsed, nil
	case int:
		return int64(number), nil
	case int64:
		return number, nil
	case float64:
		if number < -9223372036854775808.0 || number >= 9223372036854775808.0 || math.Trunc(number) != number {
			return 0, fmt.Errorf("must be an integer")
		}
		return int64(number), nil
	default:
		return 0, fmt.Errorf("must be an integer")
	}
}

func normalizeUnsigned(value any) (uint64, error) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseUint(number.String(), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("must be an unsigned 64-bit integer")
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseUint(number, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("must be an unsigned 64-bit integer")
		}
		return parsed, nil
	case uint:
		return uint64(number), nil
	case uint64:
		return number, nil
	case int:
		if number < 0 {
			return 0, fmt.Errorf("must be an unsigned integer")
		}
		return uint64(number), nil
	case int64:
		if number < 0 {
			return 0, fmt.Errorf("must be an unsigned integer")
		}
		return uint64(number), nil
	case float64:
		if number < 0 || number >= 18446744073709551616.0 || math.Trunc(number) != number {
			return 0, fmt.Errorf("must be an unsigned integer")
		}
		return uint64(number), nil
	default:
		return 0, fmt.Errorf("must be an unsigned integer")
	}
}

func normalizeBoolean(value any) (bool, error) {
	boolean, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("must be a boolean")
	}
	return boolean, nil
}

func normalizeJSON(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("must be valid JSON")
	}
	return json.RawMessage(encoded), nil
}

func normalizeTime(value any) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("must be an RFC3339 timestamp")
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be an RFC3339 timestamp")
	}
	return parsed.UTC(), nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
