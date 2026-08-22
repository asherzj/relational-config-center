// Package bootstrap wires the first built-in managed-table policies.
package bootstrap

import "github.com/asherzj/relational-config-center/admin/internal/domain"

// Registry returns all tables intentionally exposed by Admin.
func Registry() (*domain.Registry, error) {
	return domain.NewRegistry(configsPolicy())
}

func configsPolicy() domain.Policy {
	commonComparison := []domain.Operator{
		domain.OperatorEqual,
		domain.OperatorNotEqual,
		domain.OperatorIn,
	}
	orderedComparison := []domain.Operator{
		domain.OperatorEqual,
		domain.OperatorNotEqual,
		domain.OperatorIn,
		domain.OperatorGreaterThan,
		domain.OperatorGreaterThanOrEqual,
		domain.OperatorLessThan,
		domain.OperatorLessThanOrEqual,
	}
	return domain.Policy{
		Resource:    "configs",
		Table:       "configs",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		DefaultSort: []domain.Sort{{
			Field:     "id",
			Direction: domain.DirectionDescending,
		}},
		DefaultPageSize: 20,
		MaxPageSize:     100,
		MaxFilterDepth:  4,
		MaxFilterNodes:  32,
		MaxInValues:     100,
		Fields: map[string]domain.FieldPolicy{
			"id": {
				Column:          "id",
				Type:            domain.TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				AutoIncrement:   true,
				FilterOperators: orderedComparison,
			},
			"namespace": {
				Column:           "namespace",
				Type:             domain.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        128,
				FilterOperators:  append(append([]domain.Operator(nil), commonComparison...), domain.OperatorContains),
			},
			"key": {
				Column:           "config_key",
				Type:             domain.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        255,
				FilterOperators:  append(append([]domain.Operator(nil), commonComparison...), domain.OperatorContains),
			},
			"value": {
				Column:           "config_value",
				Type:             domain.TypeJSON,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
			},
			"status": {
				Column:          "status",
				Type:            domain.TypeString,
				Readable:        true,
				Creatable:       true,
				Updatable:       true,
				Sortable:        true,
				MaxLength:       32,
				AllowedValues:   []string{"draft", "published", "archived"},
				FilterOperators: commonComparison,
			},
			"version": {
				Column:          "version",
				Type:            domain.TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				FilterOperators: orderedComparison,
			},
			"created_at": {
				Column:          "created_at",
				Type:            domain.TypeTime,
				Readable:        true,
				Sortable:        true,
				FilterOperators: orderedComparison,
			},
			"updated_at": {
				Column:          "updated_at",
				Type:            domain.TypeTime,
				Readable:        true,
				Sortable:        true,
				FilterOperators: orderedComparison,
			},
		},
	}
}
