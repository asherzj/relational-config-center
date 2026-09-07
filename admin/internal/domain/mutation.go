package domain

// MutationContent preserves the three HTTP mutation states: an absent key is
// omitted, a nil value is SQL NULL, and a non-nil value is a JSON String that
// must be parsed through live Schema metadata.
type MutationContent map[string]*JSONString

// MutationValue is one live-Schema-validated value ready for storage execution.
type MutationValue struct {
	Column Column
	Value  any
}

// RowInsert is the storage-neutral request passed through the mutation
// execution interface. Dynamic identifiers are live Schema values, never
// caller-provided SQL fragments.
type RowInsert struct {
	TableName  string
	Values     []MutationValue
	ProvidedID *JSONString
}

// RowUpdate is one live-Schema-validated PATCH operation. The primary key and
// values are typed before this storage-neutral request crosses the execution
// seam, so its adapter only owns SQL compilation and transaction handling.
type RowUpdate struct {
	ExpectedVersion string
	TableName       string
	IDColumn        Column
	ID              any
	Values          []MutationValue
}

// RowDelete is one live-Schema-validated hard delete. Only the table and sole
// id primary key cross the execution interface; unrelated columns are not part
// of DELETE validation.
type RowDelete struct {
	ExpectedVersion string
	TableName       string
	IDColumn        Column
	ID              any
}
