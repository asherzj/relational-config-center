package domain

import "testing"

func TestPolicyRejectsUnsafePrimaryKey(t *testing.T) {
	policy := testPolicy()
	policy.Fields["id"] = FieldPolicy{Column: "id", Type: TypeUnsigned, Readable: true}
	if err := policy.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want non-auto-increment key requirements error")
	}
}

func TestPolicyRejectsUnsafePhysicalIdentifier(t *testing.T) {
	policy := testPolicy()
	policy.Table = "app_widgets; DROP TABLE users"
	if err := policy.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe table identifier error")
	}
}

func testPolicy() Policy {
	return Policy{
		Resource:    "widgets",
		Table:       "app_widgets",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		Fields: map[string]FieldPolicy{
			"id": {
				Column:          "widget_id",
				Type:            TypeUnsigned,
				Readable:        true,
				Sortable:        true,
				AutoIncrement:   true,
				FilterOperators: []Operator{OperatorEqual, OperatorIn},
			},
			"name": {
				Column:           "widget_name",
				Type:             TypeString,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				Sortable:         true,
				RequiredOnCreate: true,
				MinLength:        1,
				MaxLength:        32,
				FilterOperators:  []Operator{OperatorEqual, OperatorContains},
			},
			"data": {
				Column:           "widget_data",
				Type:             TypeJSON,
				Readable:         true,
				Creatable:        true,
				Updatable:        true,
				RequiredOnCreate: true,
			},
		},
	}
}
