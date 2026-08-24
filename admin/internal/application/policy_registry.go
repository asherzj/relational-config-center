package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

const (
	MySQLPageQueryV1           = "mysql_page_query_v1"
	MySQLSingleTableMutationV1 = "mysql_single_table_mutation_v1"
)

var (
	ErrUnknownQueryStrategy    = errors.New("unknown Query Policy strategy")
	ErrUnknownMutationStrategy = errors.New("unknown Mutation Policy strategy")
	ErrInvalidPolicyConfig     = errors.New("invalid Policy configuration")
)

// QueryStrategy is an opaque, freshly constructed Query Policy strategy.
type QueryStrategy interface {
	queryStrategy()
	validateSchema(domain.TableSchema) error
	execute(context.Context, domain.TableSchema, domain.QuerySpec, QueryExecutor) (domain.QueryResult, error)
}

// MutationStrategy is an opaque, freshly constructed Mutation Policy strategy.
type MutationStrategy interface {
	mutationStrategy()
	validateSchema(domain.TableSchema) error
	add(context.Context, domain.TableSchema, domain.MutationContent, OperatorProvider, MutationExecutor) (string, error)
	modify(context.Context, domain.TableSchema, domain.JSONString, domain.MutationContent, OperatorProvider, MutationExecutor) (int64, error)
	delete(context.Context, domain.TableSchema, domain.JSONString, MutationExecutor) (int64, error)
}

type QueryConstructor func(json.RawMessage) (QueryStrategy, error)
type MutationConstructor func(json.RawMessage) (MutationStrategy, error)

type QueryRegistration struct {
	ID          string
	Constructor QueryConstructor
}

type MutationRegistration struct {
	ID          string
	Constructor MutationConstructor
}

// StrategyRegistry is immutable after construction. It hides constructor
// lookup and typed configuration decoding behind one small interface.
type StrategyRegistry struct {
	queries   map[string]QueryConstructor
	mutations map[string]MutationConstructor
}

func NewStrategyRegistry(queryRegistrations []QueryRegistration, mutationRegistrations []MutationRegistration) (*StrategyRegistry, error) {
	registry := &StrategyRegistry{
		queries:   make(map[string]QueryConstructor, len(queryRegistrations)),
		mutations: make(map[string]MutationConstructor, len(mutationRegistrations)),
	}
	for _, registration := range queryRegistrations {
		if registration.ID == "" || registration.Constructor == nil {
			return nil, fmt.Errorf("invalid Query Policy registration")
		}
		if _, exists := registry.queries[registration.ID]; exists {
			return nil, fmt.Errorf("duplicate Query Policy strategy %q", registration.ID)
		}
		registry.queries[registration.ID] = registration.Constructor
	}
	for _, registration := range mutationRegistrations {
		if registration.ID == "" || registration.Constructor == nil {
			return nil, fmt.Errorf("invalid Mutation Policy registration")
		}
		if _, exists := registry.mutations[registration.ID]; exists {
			return nil, fmt.Errorf("duplicate Mutation Policy strategy %q", registration.ID)
		}
		registry.mutations[registration.ID] = registration.Constructor
	}
	return registry, nil
}

func (registry *StrategyRegistry) Validate(queryID string, queryConfig json.RawMessage, mutationID string, mutationConfig json.RawMessage) error {
	if _, err := registry.Query(queryID, queryConfig); err != nil {
		return err
	}
	if _, err := registry.Mutation(mutationID, mutationConfig); err != nil {
		return err
	}
	return nil
}

func (registry *StrategyRegistry) ValidateForSchema(queryID string, queryConfig json.RawMessage, mutationID string, mutationConfig json.RawMessage, schema domain.TableSchema) error {
	query, err := registry.Query(queryID, queryConfig)
	if err != nil {
		return err
	}
	mutation, err := registry.Mutation(mutationID, mutationConfig)
	if err != nil {
		return err
	}
	if err := query.validateSchema(schema); err != nil {
		return err
	}
	return mutation.validateSchema(schema)
}

func (registry *StrategyRegistry) Query(id string, config json.RawMessage) (QueryStrategy, error) {
	constructor, found := registry.queries[id]
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrUnknownQueryStrategy, id)
	}
	return constructor(config)
}

func (registry *StrategyRegistry) Mutation(id string, config json.RawMessage) (MutationStrategy, error) {
	constructor, found := registry.mutations[id]
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrUnknownMutationStrategy, id)
	}
	return constructor(config)
}

type pageQueryStrategy struct {
	config pageQueryConfig
}

func (*pageQueryStrategy) queryStrategy() {}

func (strategy *pageQueryStrategy) validateSchema(schema domain.TableSchema) error {
	for _, column := range schema.Columns {
		if column.Type == domain.ColumnTypeUnsupported {
			return fmt.Errorf("%w: unsupported live column %s", ErrIncompatibleTable, column.Name)
		}
	}
	if _, found := schema.Column(strategy.config.DefaultOrder.Field); !found {
		return fmt.Errorf("%w: default_order.field does not exist", ErrInvalidPolicyConfig)
	}
	return nil
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
	offset := pageIndex * pageSize
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
		TableName:  schema.Name,
		Columns:    append([]domain.Column(nil), schema.Columns...),
		Conditions: conditions,
		Order:      order,
		PageNumber: pageNumber,
		PageSize:   pageSize,
		Offset:     offset,
	})
}

type pageQueryConfig struct {
	DefaultOrder    *defaultOrder `json:"default_order,omitempty"`
	DefaultPageSize int           `json:"default_page_size,omitempty"`
	MaxPageSize     int           `json:"max_page_size,omitempty"`
}

type defaultOrder struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

func NewMySQLPageQueryStrategy(raw json.RawMessage) (QueryStrategy, error) {
	var config pageQueryConfig
	if err := decodeStrictObject(raw, &config); err != nil {
		return nil, fmt.Errorf("%w: query_policy_config: %v", ErrInvalidPolicyConfig, err)
	}
	if config.DefaultOrder != nil {
		if strings.TrimSpace(config.DefaultOrder.Field) == "" {
			return nil, fmt.Errorf("%w: default_order.field is required", ErrInvalidPolicyConfig)
		}
		if config.DefaultOrder.Direction != "ASC" && config.DefaultOrder.Direction != "DESC" {
			return nil, fmt.Errorf("%w: default_order.direction must be ASC or DESC", ErrInvalidPolicyConfig)
		}
	} else {
		config.DefaultOrder = &defaultOrder{Field: "id", Direction: "DESC"}
	}
	if config.DefaultPageSize < 0 || config.MaxPageSize < 0 {
		return nil, fmt.Errorf("%w: page sizes cannot be negative", ErrInvalidPolicyConfig)
	}
	if config.MaxPageSize > 200 {
		return nil, fmt.Errorf("%w: max_page_size cannot exceed 200", ErrInvalidPolicyConfig)
	}
	if config.DefaultPageSize > 0 && config.MaxPageSize > 0 && config.DefaultPageSize > config.MaxPageSize {
		return nil, fmt.Errorf("%w: default_page_size cannot exceed max_page_size", ErrInvalidPolicyConfig)
	}
	if config.DefaultPageSize == 0 {
		config.DefaultPageSize = 20
	}
	if config.MaxPageSize == 0 {
		config.MaxPageSize = 200
	}
	if config.DefaultPageSize > config.MaxPageSize {
		return nil, fmt.Errorf("%w: default_page_size cannot exceed max_page_size", ErrInvalidPolicyConfig)
	}
	return &pageQueryStrategy{config: config}, nil
}

type singleTableMutationStrategy struct {
	config singleTableMutationConfig
}

func (*singleTableMutationStrategy) mutationStrategy() {}

func (strategy *singleTableMutationStrategy) validateSchema(schema domain.TableSchema) error {
	if strategy.config.AutoFill == nil {
		return nil
	}
	for operation, rules := range map[string]map[string]autoFillRule{"add": strategy.config.AutoFill.Add, "modify": strategy.config.AutoFill.Modify} {
		for field := range rules {
			if operation == "modify" && field == "id" {
				return fmt.Errorf("%w: auto_fill.modify.id cannot change the primary key", ErrInvalidPolicyConfig)
			}
			column, found := schema.Column(field)
			if !found {
				return fmt.Errorf("%w: auto_fill.%s.%s does not exist", ErrInvalidPolicyConfig, operation, field)
			}
			if !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
				return fmt.Errorf("%w: auto_fill.%s.%s is not writable", ErrInvalidPolicyConfig, operation, field)
			}
		}
	}
	return nil
}

func (strategy *singleTableMutationStrategy) add(ctx context.Context, schema domain.TableSchema, content domain.MutationContent, operator OperatorProvider, executor MutationExecutor) (string, error) {
	effective := make(domain.MutationContent, len(content))
	for field, value := range content {
		effective[field] = value
	}
	if strategy.config.AutoFill != nil {
		for field, rule := range strategy.config.AutoFill.Add {
			column, found := schema.Column(field)
			if !found || !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
				return "", ErrInvalidMutation
			}
			value, err := autoFillValue(ctx, column, rule, operator)
			if err != nil {
				return "", err
			}
			effective[field] = &value
		}
	}

	values := make([]domain.MutationValue, 0, len(effective))
	for field, value := range effective {
		column, found := schema.Column(field)
		if !found || !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
			return "", ErrInvalidMutation
		}
		if value == nil {
			if !column.Nullable {
				return "", ErrInvalidMutation
			}
			values = append(values, domain.MutationValue{Column: column})
			continue
		}
		parsed, err := domain.ParseColumnValue(column, *value)
		if err != nil {
			return "", ErrInvalidMutation
		}
		values = append(values, domain.MutationValue{Column: column, Value: parsed})
	}
	for _, column := range schema.Columns {
		if _, supplied := effective[column.Name]; column.RequiredForInsert() && !supplied {
			return "", ErrMissingRequiredField
		}
	}
	return executor.InsertRow(ctx, domain.RowInsert{TableName: schema.Name, Values: values, ProvidedID: effective["id"]})
}

func (strategy *singleTableMutationStrategy) modify(ctx context.Context, schema domain.TableSchema, id domain.JSONString, content domain.MutationContent, operator OperatorProvider, executor MutationExecutor) (int64, error) {
	if _, supplied := content["id"]; supplied {
		return 0, ErrInvalidMutation
	}
	idColumn, found := schema.Column("id")
	if !found {
		return 0, ErrIncompatibleTable
	}
	parsedID, err := domain.ParseColumnValue(idColumn, id)
	if err != nil {
		return 0, ErrInvalidMutation
	}

	effective := make(domain.MutationContent, len(content))
	for field, value := range content {
		effective[field] = value
	}
	if strategy.config.AutoFill != nil {
		for field, rule := range strategy.config.AutoFill.Modify {
			if field == "id" {
				return 0, ErrInvalidPolicyConfig
			}
			column, found := schema.Column(field)
			if !found || !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
				return 0, ErrInvalidMutation
			}
			value, err := autoFillValue(ctx, column, rule, operator)
			if err != nil {
				return 0, err
			}
			effective[field] = &value
		}
	}
	if len(effective) == 0 {
		return 0, ErrInvalidMutation
	}

	values := make([]domain.MutationValue, 0, len(effective))
	for field, value := range effective {
		if field == "id" {
			return 0, ErrInvalidMutation
		}
		column, found := schema.Column(field)
		if !found || !column.Writable() || column.Type == domain.ColumnTypeUnsupported {
			return 0, ErrInvalidMutation
		}
		if value == nil {
			if !column.Nullable {
				return 0, ErrInvalidMutation
			}
			values = append(values, domain.MutationValue{Column: column})
			continue
		}
		parsed, err := domain.ParseColumnValue(column, *value)
		if err != nil {
			return 0, ErrInvalidMutation
		}
		values = append(values, domain.MutationValue{Column: column, Value: parsed})
	}
	return executor.UpdateRow(ctx, domain.RowUpdate{TableName: schema.Name, IDColumn: idColumn, ID: parsedID, Values: values})
}

func (strategy *singleTableMutationStrategy) delete(ctx context.Context, schema domain.TableSchema, id domain.JSONString, executor MutationExecutor) (int64, error) {
	idColumn, found := schema.Column("id")
	if !found || idColumn.Type == domain.ColumnTypeUnsupported {
		return 0, ErrIncompatibleTable
	}
	parsedID, err := domain.ParseColumnValue(idColumn, id)
	if err != nil {
		return 0, ErrInvalidMutation
	}
	return executor.DeleteRow(ctx, domain.RowDelete{TableName: schema.Name, IDColumn: idColumn, ID: parsedID})
}

func autoFillValue(ctx context.Context, column domain.Column, rule autoFillRule, operator OperatorProvider) (domain.JSONString, error) {
	switch rule.Source {
	case "operator":
		value, err := operator.Operator(ctx)
		if err != nil {
			return "", ErrMutationUnavailable
		}
		return value, nil
	case "now":
		return currentTimeValue(column, time.Now().UTC())
	case "literal":
		return domain.JSONString(*rule.Value), nil
	default:
		return "", ErrInvalidMutation
	}
}

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

type singleTableMutationConfig struct {
	AutoFill *autoFillConfig `json:"auto_fill,omitempty"`
}

type autoFillConfig struct {
	Add    map[string]autoFillRule `json:"add,omitempty"`
	Modify map[string]autoFillRule `json:"modify,omitempty"`
}

type autoFillRule struct {
	Source string  `json:"source"`
	Value  *string `json:"value,omitempty"`
}

func NewMySQLSingleTableMutationStrategy(raw json.RawMessage) (MutationStrategy, error) {
	var config singleTableMutationConfig
	if err := decodeStrictObject(raw, &config); err != nil {
		return nil, fmt.Errorf("%w: mutation_policy_config: %v", ErrInvalidPolicyConfig, err)
	}
	if config.AutoFill != nil {
		for operation, rules := range map[string]map[string]autoFillRule{"add": config.AutoFill.Add, "modify": config.AutoFill.Modify} {
			for field, rule := range rules {
				if strings.TrimSpace(field) == "" {
					return nil, fmt.Errorf("%w: auto_fill.%s field is empty", ErrInvalidPolicyConfig, operation)
				}
				switch rule.Source {
				case "operator", "now":
					if rule.Value != nil {
						return nil, fmt.Errorf("%w: auto_fill.%s.%s value is only valid for literal", ErrInvalidPolicyConfig, operation, field)
					}
				case "literal":
					if rule.Value == nil {
						return nil, fmt.Errorf("%w: auto_fill.%s.%s literal value is required", ErrInvalidPolicyConfig, operation, field)
					}
				default:
					return nil, fmt.Errorf("%w: auto_fill.%s.%s has unknown source", ErrInvalidPolicyConfig, operation, field)
				}
			}
		}
	}
	return &singleTableMutationStrategy{config: config}, nil
}

func decodeStrictObject(raw json.RawMessage, destination any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' {
		return errors.New("must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return err
	}
	return nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("contains trailing JSON")
}
