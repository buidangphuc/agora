package postgres_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestLocalOnlyDatabaseFallbackRemoved(t *testing.T) {
	removed := "OpenOr" + "Local"
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, pkg := range pkgs {
		for name, f := range pkg.Files {
			for _, d := range f.Decls {
				if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == removed {
					t.Errorf("%s declares %s; the local-only database fallback must be removed", name, removed)
				}
			}
		}
	}
}
