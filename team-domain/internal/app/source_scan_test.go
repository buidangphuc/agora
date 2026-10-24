package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const storetestImport = "github.com/buidangphuc/team-domain/internal/store/storetest"

type sourceFile struct {
	rel  string
	path string
}

func walkGo(t *testing.T) []sourceFile {
	t.Helper()
	root := serviceRoot(t)
	var out []sourceFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			switch rel {
			case "generated", "proto", "node_modules", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") {
			out = append(out, sourceFile{rel: filepath.ToSlash(rel), path: p})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func isTestFile(rel string) bool { return strings.HasSuffix(rel, "_test.go") }

func inStoretest(rel string) bool { return strings.HasPrefix(rel, "internal/store/storetest/") }

func TestFakesImportedOnlyFromTests(t *testing.T) {
	dir := filepath.Join(serviceRoot(t), "internal", "store", "storetest")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("internal/store/storetest must exist: %v", err)
	}
	fakeFiles := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			fakeFiles++
		}
	}
	if fakeFiles == 0 {
		t.Fatalf("internal/store/storetest holds no non-test Go file: the in-memory fakes must live there")
	}

	importers := 0
	fset := token.NewFileSet()
	for _, f := range walkGo(t) {
		parsed, err := parser.ParseFile(fset, f.path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", f.rel, err)
			continue
		}
		for _, imp := range parsed.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if path != storetestImport {
				continue
			}
			importers++
			if !isTestFile(f.rel) {
				t.Errorf("%s imports %s but is not a _test.go file", f.rel, storetestImport)
			}
		}
	}
	if importers == 0 {
		t.Errorf("no test file imports %s: the fakes are not used", storetestImport)
	}
}

var bannedTokens = []string{
	"NoopPublisher", "NewInMemory", "RequireDurableStorage",
	"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "IDENTITY_CONSUMER_ENABLED",
}

var dependencyNames = map[string]bool{
	"screener": true, "screen": true, "audit": true, "auditor": true, "auditwriter": true, "auditclient": true,
	"pool": true, "producer": true, "publisher": true, "aiclient": true, "teamai": true,
}

func trailingName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return strings.ToLower(x.Name)
	case *ast.SelectorExpr:
		return strings.ToLower(x.Sel.Name)
	case *ast.ParenExpr:
		return trailingName(x.X)
	case *ast.StarExpr:
		return trailingName(x.X)
	}
	return ""
}

func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func TestNoRuntimeFallbacksInSource(t *testing.T) {
	fset := token.NewFileSet()
	scanned := 0
	for _, f := range walkGo(t) {
		if isTestFile(f.rel) || inStoretest(f.rel) {
			continue
		}
		scanned++
		raw, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatalf("read %s: %v", f.rel, err)
		}
		for _, tok := range bannedTokens {
			if strings.Contains(string(raw), tok) {
				t.Errorf("%s contains %q", f.rel, tok)
			}
		}
		parsed, err := parser.ParseFile(fset, f.path, raw, 0)
		if err != nil {
			t.Errorf("parse %s: %v", f.rel, err)
			continue
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
				return true
			}
			var other ast.Expr
			switch {
			case isNil(be.Y):
				other = be.X
			case isNil(be.X):
				other = be.Y
			default:
				return true
			}
			if name := trailingName(other); dependencyNames[name] {
				t.Errorf("%s:%d compares dependency %q with nil: a dependency must always be wired", f.rel, fset.Position(be.Pos()).Line, name)
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("scanned no non-test source file")
	}
}
