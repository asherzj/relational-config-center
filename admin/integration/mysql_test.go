package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/gorm"
)

func TestManagedTableFlowAgainstMySQL84(t *testing.T) {
	ctx, db := startMySQL84(t)
	service := serviceFromCatalog(t, ctx, db)

	first, err := service.Create(ctx, "configs", map[string]any{
		"namespace": "default",
		"key":       "discount_%",
		"value":     map[string]any{"percent": 10},
		"status":    "draft",
	})
	if err != nil {
		t.Fatalf("create first row: %v", err)
	}
	if first.Key == "" || first.AffectedRows != 1 {
		t.Fatalf("first create result = %+v", first)
	}
	second, err := service.Create(ctx, "configs", map[string]any{
		"namespace": "default",
		"key":       "discount-ab",
		"value":     map[string]any{"percent": 20},
		"status":    "draft",
	})
	if err != nil {
		t.Fatalf("create second row: %v", err)
	}

	page, err := service.Query(ctx, "configs", domain.QuerySpec{
		Filter: &domain.Filter{
			Field:    "key",
			Operator: domain.OperatorContains,
			Value:    "discount_%",
		},
		Sort: []domain.Sort{{Field: "id", Direction: domain.DirectionAscending}},
		Page: domain.Page{Number: 1, Size: 1},
	})
	if err != nil {
		t.Fatalf("query escaped contains page: %v", err)
	}
	if page.Page.Total != 1 || len(page.Rows) != 1 || page.Rows[0]["key"] != "discount_%" {
		t.Fatalf("escaped contains page = %+v", page)
	}
	value, ok := page.Rows[0]["value"].(map[string]any)
	if !ok || value["percent"] != json.Number("10") {
		t.Fatalf("decoded JSON value = %#v", page.Rows[0]["value"])
	}

	updated, err := service.Update(ctx, "configs", first.Key, map[string]any{"status": "published"})
	if err != nil || updated.AffectedRows != 1 {
		t.Fatalf("update row = %+v, %v", updated, err)
	}
	updated, err = service.Update(ctx, "configs", first.Key, map[string]any{"status": "published"})
	if err != nil || updated.AffectedRows != 1 {
		t.Fatalf("idempotent update row = %+v, %v", updated, err)
	}

	deleted, err := service.Delete(ctx, "configs", second.Key)
	if err != nil || deleted.AffectedRows != 1 {
		t.Fatalf("delete row = %+v, %v", deleted, err)
	}
	remaining, err := service.Query(ctx, "configs", domain.QuerySpec{})
	if err != nil {
		t.Fatalf("query remaining rows: %v", err)
	}
	if remaining.Page.Total != 1 || len(remaining.Rows) != 1 {
		t.Fatalf("remaining rows = %+v", remaining)
	}
}

// The seeded catalog row is the only source of the configs policy: editing it
// and reloading the catalog (what an Admin restart does) changes exposed
// behavior without rebuilding anything.
func TestCatalogEditChangesBehaviorWithoutRebuild(t *testing.T) {
	ctx, db := startMySQL84(t)
	service := serviceFromCatalog(t, ctx, db)

	if _, err := service.Query(ctx, "configs", domain.QuerySpec{Page: domain.Page{Number: 1, Size: 3}}); err != nil {
		t.Fatalf("page size 3 under seeded max_page_size=100: %v", err)
	}

	// Keep default_page_size <= max_page_size or Policy.Validate rejects the
	// document on reload; tightening both proves the edit took effect.
	edited := db.Exec("UPDATE table_policies SET policy = JSON_SET(policy, '$.max_page_size', 2, '$.default_page_size', 2) WHERE resource = 'configs'")
	if edited.Error != nil {
		t.Fatalf("edit catalog row: %v", edited.Error)
	}

	service = serviceFromCatalog(t, ctx, db)
	if _, err := service.Query(ctx, "configs", domain.QuerySpec{Page: domain.Page{Number: 1, Size: 3}}); !domain.IsValidation(err) {
		t.Fatalf("page size 3 after tightening max_page_size=2: error = %v, want validation error", err)
	}
	page, err := service.Query(ctx, "configs", domain.QuerySpec{Page: domain.Page{Number: 1, Size: 2}})
	if err != nil {
		t.Fatalf("page size 2 after tightening: %v", err)
	}
	if page.Page.Size != 2 {
		t.Fatalf("page size = %d, want 2", page.Page.Size)
	}
}

func startMySQL84(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	if os.Getenv("RCC_INTEGRATION") != "1" {
		t.Skip("set RCC_INTEGRATION=1 to run the MySQL container test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	container, err := tcmysql.Run(ctx,
		"mysql:8.4",
		tcmysql.WithDatabase("rcc"),
		tcmysql.WithUsername("rcc_admin"),
		tcmysql.WithPassword("rcc_admin"),
		tcmysql.WithScripts(schemaPath(t)),
	)
	if err != nil {
		t.Fatalf("start MySQL 8.4 container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate MySQL container: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "charset=utf8mb4")
	if err != nil {
		t.Fatalf("MySQL connection string: %v", err)
	}
	db, sqlDB, err := mysql.Open(ctx, mysql.Options{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return ctx, db
}

func serviceFromCatalog(t *testing.T, ctx context.Context, db *gorm.DB) *application.Service {
	t.Helper()
	_, service, _ := wiredAdmin(t, ctx, db)
	return service
}

func schemaPath(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../deploy/mysql/schema.sql"))
}

// The feature_flags table ships without a policy: saving one through the
// catalog makes it a Managed Table immediately, with no restart or rebuild.
func TestCatalogSaveTakesEffectImmediately(t *testing.T) {
	ctx, db := startMySQL84(t)
	_, service, catalog := wiredAdmin(t, ctx, db)

	if _, err := service.Query(ctx, "feature_flags", domain.QuerySpec{}); !errors.Is(err, domain.ErrUnknownResource) {
		t.Fatalf("feature_flags before policy: error = %v, want ErrUnknownResource", err)
	}

	saved, err := catalog.Save(ctx, "feature_flags", featureFlagsPolicy())
	if err != nil {
		t.Fatalf("save feature_flags policy: %v", err)
	}
	if saved.MaxPageSize != 100 {
		t.Fatalf("saved document is not canonical: %+v", saved)
	}

	page, err := service.Query(ctx, "feature_flags", domain.QuerySpec{})
	if err != nil {
		t.Fatalf("query feature_flags after save: %v", err)
	}
	if page.Page.Total != 0 {
		t.Fatalf("feature_flags page = %+v, want empty", page)
	}
	created, err := service.Create(ctx, "feature_flags", map[string]any{
		"name": "checkout-v2", "enabled": true, "rollout_percent": 25,
	})
	if err != nil || created.AffectedRows != 1 {
		t.Fatalf("create feature flag = %+v, %v", created, err)
	}
	listed := service.List()
	resources := make(map[string]bool, len(listed))
	for _, definition := range listed {
		resources[definition.Resource] = true
	}
	if !resources["configs"] || !resources["feature_flags"] {
		t.Fatalf("discovery after save = %v", resources)
	}

	if err := catalog.Delete(ctx, "feature_flags"); err != nil {
		t.Fatalf("delete feature_flags policy: %v", err)
	}
	if _, err := service.Query(ctx, "feature_flags", domain.QuerySpec{}); !errors.Is(err, domain.ErrUnknownResource) {
		t.Fatalf("feature_flags after delete: error = %v, want ErrUnknownResource", err)
	}
}

// Saving a policy that references a nonexistent physical table or column is
// rejected with a clear error and nothing is persisted.
func TestCatalogSaveRejectsUnknownPhysicalSchema(t *testing.T) {
	ctx, db := startMySQL84(t)
	_, _, catalog := wiredAdmin(t, ctx, db)

	missingTable := featureFlagsPolicy()
	missingTable.Table = "missing_table"
	_, err := catalog.Save(ctx, "feature_flags", missingTable)
	if !domain.IsValidation(err) || !strings.Contains(err.Error(), "missing_table") {
		t.Fatalf("missing table error = %v", err)
	}

	missingColumn := featureFlagsPolicy()
	missingColumn.Fields["name"] = domain.FieldPolicy{
		Column: "missing_column", Type: domain.TypeString,
		Readable: true, Creatable: true, Updatable: true, RequiredOnCreate: true,
	}
	_, err = catalog.Save(ctx, "feature_flags", missingColumn)
	if !domain.IsValidation(err) || !strings.Contains(err.Error(), "missing_column") {
		t.Fatalf("missing column error = %v", err)
	}

	policies, listErr := catalog.List(ctx)
	if listErr != nil || len(policies) != 1 || policies[0].Resource != "configs" {
		t.Fatalf("catalog after rejected saves = %+v, %v", policies, listErr)
	}
}

func wiredAdmin(t *testing.T, ctx context.Context, db *gorm.DB) (*application.PolicySource, *application.Service, *application.CatalogService) {
	t.Helper()
	policies, err := mysql.NewPolicyCatalog(db).Load(ctx)
	if err != nil {
		t.Fatalf("load table policies: %v", err)
	}
	registry, err := domain.NewRegistry(policies...)
	if err != nil {
		t.Fatalf("build table policy registry: %v", err)
	}
	source := application.NewPolicySource(registry)
	service := application.NewService(source, mysql.NewRepository(db))
	catalog := application.NewCatalogService(mysql.NewPolicyCatalog(db), mysql.NewSchemaVerifier(db), source)
	return source, service, catalog
}

func featureFlagsPolicy() domain.Policy {
	return domain.Policy{
		Resource:    "feature_flags",
		Table:       "feature_flags",
		PrimaryKey:  "id",
		AllowCreate: true,
		AllowUpdate: true,
		AllowDelete: true,
		Fields: map[string]domain.FieldPolicy{
			"id": {
				Column: "id", Type: domain.TypeUnsigned,
				Readable: true, Sortable: true, AutoIncrement: true,
				FilterOperators: []domain.Operator{domain.OperatorEqual, domain.OperatorIn},
			},
			"name": {
				Column: "name", Type: domain.TypeString,
				Readable: true, Creatable: true, Updatable: true, Sortable: true,
				RequiredOnCreate: true, MinLength: 1, MaxLength: 128,
				FilterOperators: []domain.Operator{domain.OperatorEqual, domain.OperatorContains},
			},
			"enabled": {
				Column: "enabled", Type: domain.TypeBoolean,
				Readable: true, Creatable: true, Updatable: true,
			},
			"rollout_percent": {
				Column: "rollout_percent", Type: domain.TypeUnsigned,
				Readable: true, Creatable: true, Updatable: true,
			},
			"created_at": {Column: "created_at", Type: domain.TypeTime, Readable: true},
			"updated_at": {Column: "updated_at", Type: domain.TypeTime, Readable: true},
		},
	}
}
