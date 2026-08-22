package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/bootstrap"
	"github.com/asherzj/relational-config-center/admin/internal/managedtable"
	"github.com/asherzj/relational-config-center/admin/internal/mysqlstore"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestManagedTableFlowAgainstMySQL84(t *testing.T) {
	if os.Getenv("RCC_INTEGRATION") != "1" {
		t.Skip("set RCC_INTEGRATION=1 to run the MySQL container test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
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
	db, sqlDB, err := mysqlstore.Open(ctx, mysqlstore.Options{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	registry, err := bootstrap.Registry()
	if err != nil {
		t.Fatalf("build policy registry: %v", err)
	}
	service := managedtable.NewService(registry, mysqlstore.NewRepository(db))

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

	page, err := service.Query(ctx, "configs", managedtable.QuerySpec{
		Filter: &managedtable.Filter{
			Field:    "key",
			Operator: managedtable.OperatorContains,
			Value:    "discount_%",
		},
		Sort: []managedtable.Sort{{Field: "id", Direction: managedtable.DirectionAscending}},
		Page: managedtable.Page{Number: 1, Size: 1},
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
	remaining, err := service.Query(ctx, "configs", managedtable.QuerySpec{})
	if err != nil {
		t.Fatalf("query remaining rows: %v", err)
	}
	if remaining.Page.Total != 1 || len(remaining.Rows) != 1 {
		t.Fatalf("remaining rows = %+v", remaining)
	}
}

func schemaPath(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../deploy/mysql/schema.sql"))
}
