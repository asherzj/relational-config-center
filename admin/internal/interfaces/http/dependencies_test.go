package http

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestHTTPDependsOnApplicationRatherThanDomainOrInfrastructure(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, "/internal/") && !strings.HasSuffix(path, "/internal/application") {
				t.Errorf("%s imports %s; HTTP must consume Application contracts", entry.Name(), path)
			}
		}
	}
}

func TestSharedBusinessServicesDoNotStoreRequestIdentity(t *testing.T) {
	files, err := parser.ParseDir(token.NewFileSet(), "../../application", func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Shared business services must never regain mutable per-account fields.
	protected := map[string]bool{"QueryPolicyManagement": true, "MutationPolicyManagement": true, "TablePolicyManagement": true, "TableFieldPolicyManagement": true, "AccountRoleManagement": true, "ApprovalRoleManagement": true, "ReleaseOrders": true}
	for _, pkg := range files {
		for filename, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.TypeSpec)
				if !ok || !protected[spec.Name.Name] {
					return true
				}
				record, ok := spec.Type.(*ast.StructType)
				if !ok {
					return true
				}
				for _, field := range record.Fields.List {
					for _, name := range field.Names {
						lower := strings.ToLower(name.Name)
						if strings.Contains(lower, "operator") || strings.Contains(lower, "actor") || strings.Contains(lower, "account") || strings.Contains(lower, "session") {
							t.Errorf("%s stores request identity in shared %s.%s", filename, spec.Name.Name, name.Name)
						}
					}
				}
				return true
			})
		}
	}
}

func TestReleaseDraftSessionCannotWriteBusinessRows(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../../application/release_orders.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "ReleaseOrderSession" {
			return true
		}
		found = true
		seam := spec.Type.(*ast.InterfaceType)
		for _, field := range seam.Methods.List {
			if embedded, ok := field.Type.(*ast.Ident); ok && (embedded.Name == "PublicationSession" || embedded.Name == "MutationExecutor" || embedded.Name == "MutationSnapshotSession") {
				t.Errorf("draft session embeds business write capability %s", embedded.Name)
			}
			for _, name := range field.Names {
				switch name.Name {
				case "InsertRow", "UpdateRow", "DeleteRow", "CommitPublication":
					t.Errorf("draft session exposes business write %s", name.Name)
				}
			}
		}
		return false
	})
	if !found {
		t.Fatal("release draft transaction contract missing")
	}
}

func TestDomainDoesNotContainORMMappings(t *testing.T) {
	files, err := parser.ParseDir(token.NewFileSet(), "../../domain", func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range files {
		for name, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				if field, ok := node.(*ast.Field); ok && field.Tag != nil && strings.Contains(field.Tag.Value, "gorm:") {
					t.Errorf("%s stores ORM mapping in domain; ADR-0012 requires persistence mappings in Infrastructure", name)
				}
				return true
			})
		}
	}
}
