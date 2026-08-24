package application

import (
	"context"
	"errors"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var ErrDatabaseTableNotFound = errors.New("database table not found")

// TableMetadataReader is the inward-facing interface for live database discovery.
type TableMetadataReader interface {
	ListDatabaseTables(context.Context) ([]domain.DatabaseTable, error)
	GetDatabaseTable(context.Context, string) (domain.DatabaseTable, error)
	GetTableSchema(context.Context, string) (domain.TableSchema, error)
}

// DatabaseTableDiscovery exposes ordinary base-table discovery use cases.
type DatabaseTableDiscovery struct {
	metadata TableMetadataReader
}

func NewDatabaseTableDiscovery(metadata TableMetadataReader) *DatabaseTableDiscovery {
	return &DatabaseTableDiscovery{metadata: metadata}
}

func (discovery *DatabaseTableDiscovery) List(ctx context.Context) ([]domain.DatabaseTable, error) {
	return discovery.metadata.ListDatabaseTables(ctx)
}

func (discovery *DatabaseTableDiscovery) Get(ctx context.Context, tableName string) (domain.DatabaseTable, error) {
	return discovery.metadata.GetDatabaseTable(ctx, tableName)
}

// Readiness verifies that Admin's required infrastructure is available.
type Readiness interface {
	Ready(context.Context) error
}
