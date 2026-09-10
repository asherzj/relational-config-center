package application

import (
	"bytes"
	"encoding/json"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"math/big"
	"strings"
	"unicode/utf8"
)

type FieldPolicyValidationError struct{ FieldName, Reason string }

func (e *FieldPolicyValidationError) Error() string { return e.FieldName + ": " + e.Reason }
func (e *FieldPolicyValidationError) Unwrap() error { return ErrInvalidFieldPolicy }

func invalidField(field, message string) error {
	return &FieldPolicyValidationError{FieldName: field, Reason: message}
}
func validateFieldPolicy(p domain.TableFieldPolicy, c domain.Column) error {
	bad := func(message string) error { return invalidField(c.Name, message) }
	if strings.TrimSpace(p.DisplayName) == "" || utf8.RuneCountInString(p.DisplayName) > 200 || utf8.RuneCountInString(p.Description) > 500 {
		return bad("名称须为1至200字，说明最多500字")
	}
	if p.DisplayOrder < 0 || p.DisplayOrder > 4294967295 {
		return bad("显示顺序超出整数范围")
	}
	switch p.UIType {
	case "text", "textarea", "select", "radio":
	case "number":
		if c.Type != domain.ColumnTypeInt64 && c.Type != domain.ColumnTypeUInt64 && c.Type != domain.ColumnTypeDecimal && c.Type != domain.ColumnTypeFloat64 {
			return bad("数字控件与真实字段类型不兼容")
		}
	case "boolean":
		if c.Type != domain.ColumnTypeBoolean {
			return bad("布尔控件与真实字段类型不兼容")
		}
	case "date":
		if c.Type != domain.ColumnTypeDate {
			return bad("日期控件与真实字段类型不兼容")
		}
	case "datetime":
		if c.Type != domain.ColumnTypeDateTime && c.Type != domain.ColumnTypeTimestamp {
			return bad("日期时间控件与真实字段类型不兼容")
		}
	default:
		return bad("请选择支持的控件类型")
	}
	if p.IsQueryable && len(p.QueryOperators) == 0 {
		return bad("可查询字段至少选择一个运算符")
	}
	seenOps := map[domain.QueryOperator]bool{}
	for _, op := range p.QueryOperators {
		if seenOps[op] {
			return bad("查询运算符重复")
		}
		seenOps[op] = true
		switch op {
		case domain.QueryOperatorExact, domain.QueryOperatorIn, domain.QueryOperatorNotIn:
			if c.Type == domain.ColumnTypeUnsupported {
				return bad("真实字段类型不支持该运算符")
			}
		case domain.QueryOperatorContains:
			if !c.SupportsContains() {
				return bad("包含仅适用于文本字段")
			}
		case domain.QueryOperatorOpenRange, domain.QueryOperatorClosedRange:
			if !c.SupportsRange() {
				return bad("真实字段类型不支持范围查询")
			}
		case domain.QueryOperatorIsNull, domain.QueryOperatorIsNotNull:
			if !c.Nullable {
				return bad("非NULL字段不支持空值运算符")
			}
		default:
			return bad("未知查询运算符")
		}
	}
	if p.UIType != "select" && p.UIType != "radio" && len(p.UIOptions.Options) > 0 {
		return bad("仅下拉和单选组支持静态选项")
	}
	if p.UIType == "radio" && len(p.UIOptions.Options) == 0 {
		return bad("单选组至少需要一个选项")
	}
	if len(p.UIOptions.Options) > 1000 {
		return bad("静态选项最多1000项")
	}
	values := map[string]bool{}
	for _, option := range p.UIOptions.Options {
		if strings.TrimSpace(option.Label) == "" || utf8.RuneCountInString(option.Label) > 100 {
			return bad("选项名称须为1至100字")
		}
		if values[option.Value] {
			return bad("选项实际值重复")
		}
		values[option.Value] = true
		if _, err := domain.ParseColumnValue(c, domain.JSONString(option.Value)); err != nil {
			return bad("选项实际值与真实字段类型不兼容")
		}
		if c.TextCapacity > 0 && uint64(utf8.RuneCountInString(option.Value)) > c.TextCapacity {
			return bad("选项实际值超出字段长度")
		}
	}
	if err := validateNumberOptions(p, c); err != nil {
		return err
	}
	if p.DefaultValue != nil {
		if bytes.Equal(bytes.TrimSpace(p.DefaultValue), []byte("null")) {
			if !c.Nullable || p.IsRequired {
				return bad("预填NULL与字段空值或必填约束不兼容")
			}
		} else {
			var value string
			if err := json.Unmarshal(p.DefaultValue, &value); err != nil {
				return bad("预填值必须为无损字符串或NULL")
			}
			if _, err := domain.ParseColumnValue(c, domain.JSONString(value)); err != nil {
				return bad("预填值与真实字段类型不兼容")
			}
			if p.IsRequired && value == "" {
				return bad("必填字段不能预填空字符串")
			}
			if c.TextCapacity > 0 && uint64(utf8.RuneCountInString(value)) > c.TextCapacity {
				return bad("预填值超出字段长度")
			}
			if p.UIType == "radio" && !values[value] {
				return bad("单选组预填值须来自静态选项")
			}
		}
	}
	return nil
}

// Numeric constraints stay lossless, including values beyond IEEE-754 precision.
func validateNumberOptions(p domain.TableFieldPolicy, c domain.Column) error {
	bounds := []*string{p.UIOptions.Min, p.UIOptions.Max, p.UIOptions.Step}
	if p.UIType != "number" {
		for _, v := range bounds {
			if v != nil {
				return invalidField(c.Name, "仅数字控件支持范围和步长")
			}
		}
		return nil
	}
	parsed := make([]*big.Rat, 3)
	for i, v := range bounds {
		if v == nil {
			continue
		}
		if _, err := domain.ParseColumnValue(c, domain.JSONString(*v)); err != nil {
			return invalidField(c.Name, "数字约束与真实字段类型不兼容")
		}
		r, ok := new(big.Rat).SetString(*v)
		if !ok {
			return invalidField(c.Name, "数字约束必须为精确数字")
		}
		parsed[i] = r
	}
	if parsed[0] != nil && parsed[1] != nil && parsed[0].Cmp(parsed[1]) > 0 {
		return invalidField(c.Name, "最小值不能大于最大值")
	}
	if parsed[2] != nil && parsed[2].Sign() <= 0 {
		return invalidField(c.Name, "步长必须大于0")
	}
	if p.DefaultValue != nil && !bytes.Equal(bytes.TrimSpace(p.DefaultValue), []byte("null")) {
		var value string
		if json.Unmarshal(p.DefaultValue, &value) != nil {
			return invalidField(c.Name, "预填值必须为无损字符串")
		}
		r, ok := new(big.Rat).SetString(value)
		if !ok {
			return invalidField(c.Name, "数字预填值无效")
		}
		if parsed[0] != nil && r.Cmp(parsed[0]) < 0 || parsed[1] != nil && r.Cmp(parsed[1]) > 0 {
			return invalidField(c.Name, "预填值超出数字控件范围")
		}
		if parsed[2] != nil {
			base := new(big.Rat)
			if parsed[0] != nil {
				base.Set(parsed[0])
			}
			delta := new(big.Rat).Sub(r, base)
			if !delta.Quo(delta, parsed[2]).IsInt() {
				return invalidField(c.Name, "预填值不符合数字控件步长")
			}
		}
	}
	return nil
}
