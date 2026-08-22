package mysql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

var _ domain.SchemaVerifier = (*SchemaVerifier)(nil)

// SchemaVerifier checks policy documents against information_schema of the
// single deployment-configured database. Physical names come only from the
// policy document and are always bound as parameters.
type SchemaVerifier struct {
	db *gorm.DB
}

// NewSchemaVerifier creates a verifier bound to one database handle.
func NewSchemaVerifier(db *gorm.DB) *SchemaVerifier {
	return &SchemaVerifier{db: db}
}

// VerifyPolicy reports a clear error naming what is missing when the policy's
// physical table or any mapped column does not exist.
func (verifier *SchemaVerifier) VerifyPolicy(ctx context.Context, policy domain.Policy) error {
	var tableCount int
	if err := verifier.db.WithContext(ctx).
		Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' AND table_name = ?", policy.Table).
		Scan(&tableCount).Error; err != nil {
		return fmt.Errorf("verify table %q: %w", policy.Table, err)
	}
	if tableCount == 0 {
		return fmt.Errorf("physical table %q does not exist in the configured database", policy.Table)
	}

	var columns []string
	if err := verifier.db.WithContext(ctx).
		Raw("SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?", policy.Table).
		Scan(&columns).Error; err != nil {
		return fmt.Errorf("verify columns of table %q: %w", policy.Table, err)
	}
	existing := make(map[string]bool, len(columns))
	for _, column := range columns {
		existing[column] = true
	}
	var missing []string
	for publicName, field := range policy.Fields {
		if !existing[field.Column] {
			missing = append(missing, fmt.Sprintf("field %q maps to missing column %q", publicName, field.Column))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("physical table %q is missing columns: %s", policy.Table, strings.Join(missing, "; "))
	}
	return nil
}
