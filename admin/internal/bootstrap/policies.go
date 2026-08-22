// Package bootstrap wires the first built-in managed-table policies.
package bootstrap

import "github.com/asherzj/relational-config-center/admin/internal/managedtable"

// Registry returns all tables intentionally exposed by Admin.
func Registry() (*managedtable.Registry, error) {
	return managedtable.NewRegistry(configsPolicy())
}

func configsPolicy() managedtable.Policy {
	commonComparison := []managedtable.Operator{
		managedtable.OperatorEqual,
		managedtable.OperatorNotEqual,
		managedtable.OperatorIn,
	}
	orderedComparison := []managedtable.Operator{
		managedtable.OperatorEqual,
		managedtable.OperatorNotEqual,
		managedtable.OperatorIn,
		managedtable.OperatorGreaterThan,
		managedtable.OperatorGreaterThanOrEqual,
		managedtable.OperatorLessThan,
		managedtable.OperatorLessThanOrEqual,
	}
	return managedtable.Policy{
		Resource:    "configs",
		Table:       "configs",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		DefaultSort: []managedtable.Sort{{
			Field:     "id",
			Direction: managedtable.DirectionDescending,
		}},
		DefaultPageSize: 20,
		MaxPageSize:     100,
		MaxFilterDepth:  4,
		MaxFilterNodes:  32,
		MaxInValues:     100,
		Fields: map[string]managedtable.FieldPolicy{
			"id": {
				Column:          "id",
				Type:            managedtable.TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				AutoIncrement:   true,
				FilterOperators: orderedComparison,
			},
			"namespace": {
				Column:           "namespace",
				Type:             managedtable.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        128,
				FilterOperators:  append(append([]managedtable.Operator(nil), commonComparison...), managedtable.OperatorContains),
			},
			"key": {
				Column:           "config_key",
				Type:             managedtable.TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        255,
				FilterOperators:  append(append([]managedtable.Operator(nil), commonComparison...), managedtable.OperatorContains),
			},
			"value": {
				Column:           "config_value",
				Type:             managedtable.TypeJSON,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
			},
			"status": {
				Column:          "status",
				Type:            managedtable.TypeString,
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
				Type:            managedtable.TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				FilterOperators: orderedComparison,
			},
			"created_at": {
				Column:          "created_at",
				Type:            managedtable.TypeTime,
				Readable:        true,
				Sortable:        true,
				FilterOperators: orderedComparison,
			},
			"updated_at": {
				Column:          "updated_at",
				Type:            managedtable.TypeTime,
				Readable:        true,
				Sortable:        true,
				FilterOperators: orderedComparison,
			},
		},
	}
}
