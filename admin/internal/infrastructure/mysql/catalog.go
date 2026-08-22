package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

// policyRow maps one row of the persisted table_policies catalog table. The
// policy column stores the complete policy document as defined by the domain
// JSON tags; resource is the stable row identity.
type policyRow struct {
	Resource string
	Policy   []byte
}

func (policyRow) TableName() string { return "table_policies" }

// PolicyCatalog reads Table Policies persisted in the database. The table is
// created and seeded by deploy/mysql/schema.sql; what is in the catalog is
// entirely what the database says.
type PolicyCatalog struct {
	db *gorm.DB
}

// NewPolicyCatalog creates a catalog reader on an open GORM handle.
func NewPolicyCatalog(db *gorm.DB) *PolicyCatalog {
	return &PolicyCatalog{db: db}
}

// Load returns every persisted Table Policy in resource order. Decoding is
// strict so a malformed or hand-edited document fails startup with the
// offending resource named.
func (catalog *PolicyCatalog) Load(ctx context.Context) ([]domain.Policy, error) {
	var rows []policyRow
	if err := catalog.db.WithContext(ctx).
		Order("resource").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read table policies: %w", err)
	}
	policies := make([]domain.Policy, 0, len(rows))
	for _, row := range rows {
		var policy domain.Policy
		decoder := json.NewDecoder(bytes.NewReader(row.Policy))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&policy); err != nil {
			return nil, fmt.Errorf("decode table policy %q: %w", row.Resource, err)
		}
		policies = append(policies, policy)
	}
	return policies, nil
}
