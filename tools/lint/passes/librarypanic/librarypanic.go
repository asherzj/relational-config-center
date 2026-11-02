// Package librarypanic rejects panic in library code.
//
// A panic that takes down Admin hides the failing request behind process
// death. Library packages must return errors. Two forms are allowed because
// they do not introduce a new panic: files under cmd (the process boundary),
// tests, and rethrowing the value returned by recover so an outer recovery
// middleware still observes it.
package librarypanic

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const message = "library code must not call panic; return an error (rethrowing a recover result is allowed)"

// Analyzer reports calls to the builtin panic outside cmd and tests,
// except a rethrow of a value produced by the builtin recover.
var Analyzer = &analysis.Analyzer{
	Name: "librarypanic",
	Doc:  "library packages must not introduce panic; rethrowing recover is allowed",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if skipFile(filename) {
			continue
		}
		recovered := recoveredObjects(pass, file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isBuiltin(pass, call.Fun, "panic") {
				return true
			}
			if len(call.Args) == 1 && isRecoverRethrow(pass, call.Args[0], recovered) {
				return true
			}
			pass.Reportf(call.Pos(), "%s", message)
			return true
		})
	}
	return nil, nil
}

func skipFile(filename string) bool {
	slash := filepath.ToSlash(filename)
	if strings.HasSuffix(slash, "_test.go") {
		return true
	}
	for _, part := range strings.Split(slash, "/") {
		if part == "cmd" {
			return true
		}
	}
	return false
}

func recoveredObjects(pass *analysis.Pass, file *ast.File) map[types.Object]bool {
	found := map[types.Object]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if len(node.Lhs) == 1 && len(node.Rhs) == 1 && isBuiltinCall(pass, node.Rhs[0], "recover") {
				if id, ok := node.Lhs[0].(*ast.Ident); ok {
					if obj := pass.TypesInfo.Defs[id]; obj != nil {
						found[obj] = true
					}
				}
			}
		case *ast.ValueSpec:
			if len(node.Names) == 1 && len(node.Values) == 1 && isBuiltinCall(pass, node.Values[0], "recover") {
				if obj := pass.TypesInfo.Defs[node.Names[0]]; obj != nil {
					found[obj] = true
				}
			}
		}
		return true
	})
	return found
}

func isRecoverRethrow(pass *analysis.Pass, arg ast.Expr, recovered map[types.Object]bool) bool {
	if isBuiltinCall(pass, arg, "recover") {
		return true
	}
	id, ok := arg.(*ast.Ident)
	if !ok {
		return false
	}
	obj := pass.TypesInfo.Uses[id]
	return obj != nil && recovered[obj]
}

func isBuiltinCall(pass *analysis.Pass, expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	return ok && isBuiltin(pass, call.Fun, name)
}

func isBuiltin(pass *analysis.Pass, fun ast.Expr, name string) bool {
	id, ok := fun.(*ast.Ident)
	if !ok {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && obj.Name() == name
}
