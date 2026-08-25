package application

import (
	"context"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// QueryStrategy is a fresh executor selected by a registered Policy Type.
// It receives only relational Policy values and never decodes Catalog JSON.
type QueryStrategy interface {
	execute(context.Context, domain.TableSchema, domain.QuerySpec, QueryExecutor) (domain.QueryResult, error)
}

type pageQueryStrategy struct {
	config pageQueryConfig
}

func (strategy *pageQueryStrategy) execute(ctx context.Context, schema domain.TableSchema, spec domain.QuerySpec, executor QueryExecutor) (domain.QueryResult, error) {
	pageNumber := spec.PageNumber
	if pageNumber == 0 {
		pageNumber = 1
	}
	if pageNumber < 1 {
		return domain.QueryResult{}, ErrInvalidPagination
	}
	pageSize := spec.PageSize
	if pageSize == 0 {
		pageSize = strategy.config.DefaultPageSize
	}
	if pageSize < 1 || pageSize > strategy.config.MaxPageSize {
		return domain.QueryResult{}, ErrInvalidPagination
	}
	pageIndex := pageNumber - 1
	if pageIndex > 10000/pageSize {
		return domain.QueryResult{}, ErrInvalidPagination
	}
	if len(spec.Conditions) > 20 {
		return domain.QueryResult{}, ErrInvalidQueryCondition
	}

	conditions := make([]domain.QueryCondition, 0, len(spec.Conditions))
	for _, condition := range spec.Conditions {
		column, found := schema.Column(condition.Field)
		if !found {
			return domain.QueryResult{}, ErrInvalidQueryCondition
		}
		switch condition.Operator {
		case domain.QueryOperatorExact:
			if !condition.ValuePresent || condition.Value == nil || condition.FromPresent || condition.ToPresent || condition.ValuesPresent {
				return domain.QueryResult{}, ErrInvalidQueryCondition
			}
			if _, err := domain.ParseColumnValue(column, *condition.Value); err != nil {
				return domain.QueryResult{}, ErrInvalidQueryCondition
			}
		case domain.QueryOperatorContains:
			if !condition.ValuePresent || condition.Value == nil || condition.FromPresent || condition.ToPresent || condition.ValuesPresent || !column.SupportsContains() {
				return domain.QueryResult{}, ErrInvalidQueryCondition
			}
		case domain.QueryOperatorOpenRange, domain.QueryOperatorClosedRange:
			if condition.ValuePresent || condition.ValuesPresent || !condition.FromPresent && !condition.ToPresent || condition.FromPresent && condition.From == nil || condition.ToPresent && condition.To == nil || !column.SupportsRange() {
				return domain.QueryResult{}, ErrInvalidQueryCondition
			}
			if condition.FromPresent {
				if _, err := domain.ParseColumnValue(column, *condition.From); err != nil {
					return domain.QueryResult{}, ErrInvalidQueryCondition
				}
			}
			if condition.ToPresent {
				if _, err := domain.ParseColumnValue(column, *condition.To); err != nil {
					return domain.QueryResult{}, ErrInvalidQueryCondition
				}
			}
		case domain.QueryOperatorIn, domain.QueryOperatorNotIn:
			if condition.ValuePresent || condition.FromPresent || condition.ToPresent || !condition.ValuesPresent || len(condition.Values) == 0 || len(condition.Values) > 100 {
				return domain.QueryResult{}, ErrInvalidQueryCondition
			}
			for _, item := range condition.Values {
				if item == nil {
					return domain.QueryResult{}, ErrInvalidQueryCondition
				}
				if _, err := domain.ParseColumnValue(column, *item); err != nil {
					return domain.QueryResult{}, ErrInvalidQueryCondition
				}
			}
		case domain.QueryOperatorIsNull, domain.QueryOperatorIsNotNull:
			if condition.ValuePresent || condition.FromPresent || condition.ToPresent || condition.ValuesPresent {
				return domain.QueryResult{}, ErrInvalidQueryCondition
			}
		default:
			return domain.QueryResult{}, ErrInvalidQueryCondition
		}
		conditions = append(conditions, condition)
	}

	order := domain.QueryOrder{Field: strategy.config.DefaultOrder.Field, Direction: strategy.config.DefaultOrder.Direction}
	if spec.Order != nil {
		order = *spec.Order
	}
	if _, found := schema.Column(order.Field); !found {
		return domain.QueryResult{}, ErrInvalidQueryOrder
	}
	if order.Direction != "ASC" && order.Direction != "DESC" {
		return domain.QueryResult{}, ErrInvalidQueryOrder
	}

	return executor.ExecutePageQuery(ctx, domain.PageQuery{
		TableName: schema.Name, Columns: append([]domain.Column(nil), schema.Columns...),
		Conditions: conditions, Order: order, PageNumber: pageNumber,
		PageSize: pageSize, Offset: pageIndex * pageSize,
	})
}

type pageQueryConfig struct {
	DefaultOrder    *defaultOrder
	DefaultPageSize int
	MaxPageSize     int
}

type defaultOrder struct {
	Field     string
	Direction string
}
