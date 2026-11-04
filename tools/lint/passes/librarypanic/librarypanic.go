// Package librarypanic rejects panic in library code.
//
// A panic that takes down Admin hides the failing request behind process
// death. Library packages must return errors. Two forms are allowed because
// they do not introduce a new panic: files under cmd (the process boundary),
// tests, and rethrowing a value that still holds the result of recover so an
// outer recovery middleware still observes it. Assigning that variable again
// replaces the recovered value and ends the exemption. panic(recover()) stays
// allowed.
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
// except a direct recover rethrow or a variable whose latest write is still
// the builtin recover.
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
		// Source order matters: a recover write grants the exemption, and any
		// later write to that same variable drops it before the next panic.
		recovered := map[types.Object]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				noteAssignment(pass, node, recovered)
			case *ast.ValueSpec:
				noteValueSpec(pass, node, recovered)
			case *ast.CallExpr:
				if isBuiltin(pass, node.Fun, "panic") && !isRecoverRethrow(pass, node, recovered) {
					pass.Reportf(node.Pos(), "%s", message)
				}
			}
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

func noteAssignment(pass *analysis.Pass, stmt *ast.AssignStmt, recovered map[types.Object]bool) {
	if len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 && isBuiltinCall(pass, stmt.Rhs[0], "recover") {
		if id, ok := stmt.Lhs[0].(*ast.Ident); ok {
			if obj := objectOf(pass, id); obj != nil {
				recovered[obj] = true
			}
		}
		return
	}
	for _, lhs := range stmt.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		if obj := objectOf(pass, id); obj != nil {
			delete(recovered, obj)
		}
	}
}

func noteValueSpec(pass *analysis.Pass, spec *ast.ValueSpec, recovered map[types.Object]bool) {
	if len(spec.Names) == 1 && len(spec.Values) == 1 && isBuiltinCall(pass, spec.Values[0], "recover") {
		if obj := pass.TypesInfo.Defs[spec.Names[0]]; obj != nil {
			recovered[obj] = true
		}
	}
}

func isRecoverRethrow(pass *analysis.Pass, call *ast.CallExpr, recovered map[types.Object]bool) bool {
	if len(call.Args) != 1 {
		return false
	}
	arg := call.Args[0]
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

func objectOf(pass *analysis.Pass, id *ast.Ident) types.Object {
	if obj := pass.TypesInfo.Defs[id]; obj != nil {
		return obj
	}
	return pass.TypesInfo.Uses[id]
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
