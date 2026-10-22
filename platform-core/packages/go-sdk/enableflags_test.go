package gosdk

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Scenario: No package source reads an enable flag.
func TestNoEnableFlagInSDKSource(t *testing.T) {
	fset := token.NewFileSet()
	scanned := 0
	err := filepath.WalkDir("pkg", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		scanned++
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, uerr := strconv.Unquote(lit.Value)
			if uerr != nil {
				s = lit.Value
			}
			for _, w := range strings.FieldsFunc(s, func(r rune) bool {
				return !(r == '_' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
			}) {
				if strings.HasSuffix(w, "_ENABLED") && w != "OTEL_ENABLED" {
					t.Errorf("%s: string literal names %s", fset.Position(lit.Pos()), w)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("no source files scanned")
	}
}
