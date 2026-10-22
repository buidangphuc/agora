package postgres_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestLocalOnlyDatabaseFallbackRemoved(t *testing.T) {
	removed := "OpenOr" + "Local"
	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, name := range paths {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == removed {
				t.Errorf("%s declares %s; the local-only database fallback must be removed", name, removed)
			}
		}
	}
}
