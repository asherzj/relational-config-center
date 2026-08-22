package mysqlstore

import (
	"fmt"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/managedtable"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type queryCompiler struct{}

func (queryCompiler) applyFilter(db *gorm.DB, policy managedtable.Policy, filter *managedtable.Filter) (*gorm.DB, error) {
	if filter == nil {
		return db, nil
	}
	expression, err := compileFilter(policy, *filter)
	if err != nil {
		return nil, err
	}
	return db.Where(expression), nil
}

func compileFilter(policy managedtable.Policy, filter managedtable.Filter) (clause.Expression, error) {
	if len(filter.Items) > 0 {
		expressions := make([]clause.Expression, 0, len(filter.Items))
		for _, child := range filter.Items {
			expression, err := compileFilter(policy, child)
			if err != nil {
				return nil, err
			}
			expressions = append(expressions, expression)
		}
		if filter.Logic == managedtable.LogicAnd {
			return clause.And(expressions...), nil
		}
		if filter.Logic == managedtable.LogicOr {
			return clause.Or(expressions...), nil
		}
		return nil, fmt.Errorf("compile filter group: unsupported logic %q", filter.Logic)
	}

	field, ok := policy.Fields[filter.Field]
	if !ok {
		return nil, fmt.Errorf("compile filter: unregistered field %q", filter.Field)
	}
	column := clause.Column{Table: policy.Table, Name: field.Column}
	switch filter.Operator {
	case managedtable.OperatorEqual:
		return clause.Eq{Column: column, Value: filter.Value}, nil
	case managedtable.OperatorNotEqual:
		return clause.Neq{Column: column, Value: filter.Value}, nil
	case managedtable.OperatorIn:
		values, ok := filter.Value.([]any)
		if !ok {
			return nil, fmt.Errorf("compile filter %q: IN value is not an array", filter.Field)
		}
		return clause.IN{Column: column, Values: values}, nil
	case managedtable.OperatorContains:
		value, ok := filter.Value.(string)
		if !ok {
			return nil, fmt.Errorf("compile filter %q: contains value is not a string", filter.Field)
		}
		return containsExpression{column: column, value: value}, nil
	case managedtable.OperatorGreaterThan:
		return clause.Gt{Column: column, Value: filter.Value}, nil
	case managedtable.OperatorGreaterThanOrEqual:
		return clause.Gte{Column: column, Value: filter.Value}, nil
	case managedtable.OperatorLessThan:
		return clause.Lt{Column: column, Value: filter.Value}, nil
	case managedtable.OperatorLessThanOrEqual:
		return clause.Lte{Column: column, Value: filter.Value}, nil
	case managedtable.OperatorIsNull:
		isNull, ok := filter.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("compile filter %q: is_null value is not a boolean", filter.Field)
		}
		if isNull {
			return clause.Eq{Column: column, Value: nil}, nil
		}
		return clause.Neq{Column: column, Value: nil}, nil
	default:
		return nil, fmt.Errorf("compile filter: unsupported operator %q", filter.Operator)
	}
}

// containsExpression emits a trusted identifier and a bound LIKE value. The
// explicit escape character makes %, _, and ! literal client input.
type containsExpression struct {
	column clause.Column
	value  string
}

func (expression containsExpression) Build(builder clause.Builder) {
	builder.WriteQuoted(expression.column)
	builder.WriteString(" LIKE ")
	builder.AddVar(builder, "%"+escapeLike(expression.value)+"%")
	builder.WriteString(" ESCAPE '!'")
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return replacer.Replace(value)
}
