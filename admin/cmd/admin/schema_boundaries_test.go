package main_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Deployment maintenance must remain separate from the normal HTTP composition
// root even though both executables use the same MySQL adapter.
func TestAdminHTTPCompositionCannotRunSchemaMaintenance(t *testing.T) {
	files, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range files {
		for path, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch selector.Sel.Name {
				case "OpenMaintenance", "MigrateControlSchema", "BaselineControlSchema":
					t.Errorf("%s routes HTTP startup through maintenance operation %s", path, selector.Sel.Name)
				}
				return true
			})
		}
	}
}

func TestCurrentSchemaEntryPointsDoNotRestoreRetiredInitialization(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	retired := filepath.Join(root, "deploy", "mysql", "init")
	if entries, err := os.ReadDir(retired); err == nil && len(entries) > 0 {
		t.Fatal("retired current initialization directory has returned")
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, relative := range []string{"deploy/docker-compose.yml", "scripts/browser-acceptance.sh", "README.md"} {
		data, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "mysql/init/") {
			t.Errorf("%s uses the retired initialization path", relative)
		}
	}
}
