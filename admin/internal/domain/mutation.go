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
