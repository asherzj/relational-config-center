package domain

// IncompatibilityReason is a stable explanation of why an ordinary base table
// cannot become a Managed Table.
type IncompatibilityReason string

const (
	IncompatibleMissingPrimaryKey IncompatibilityReason = "missing_primary_key"
	IncompatibleCompositeKey      IncompatibilityReason = "composite_primary_key"
	IncompatiblePrimaryKeyName    IncompatibilityReason = "primary_key_must_be_id"
)

// DatabaseTable describes a Policy candidate discovered from the Managed Data Source.
type DatabaseTable struct {
	Name                  string
	Comment               string
	PolicyExists          bool
	PolicyEnabled         bool
	Compatible            bool
	IncompatibilityReason *IncompatibilityReason
}

// DescribeDatabaseTable applies the Managed Table identity invariant to live metadata.
func DescribeDatabaseTable(name, comment string, primaryKey []string, policyExists, policyEnabled bool) DatabaseTable {
	table := DatabaseTable{
		Name:          name,
		Comment:       comment,
		PolicyExists:  policyExists,
		PolicyEnabled: policyEnabled,
		Compatible:    true,
	}

	var reason IncompatibilityReason
	switch {
	case len(primaryKey) == 0:
		reason = IncompatibleMissingPrimaryKey
	case len(primaryKey) > 1:
		reason = IncompatibleCompositeKey
	case primaryKey[0] != "id":
		reason = IncompatiblePrimaryKeyName
	default:
		return table
	}

	table.Compatible = false
	table.IncompatibilityReason = &reason
	return table
}
