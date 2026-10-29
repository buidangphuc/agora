package app_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const sdkModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"

func serviceRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve service root: %v", err)
	}
	return root
}

func TestModuleRequiresSDKTag(t *testing.T) {
	root := serviceRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	requireRe := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+v1\.0\.2\s*(?://.*)?$`)
	if !requireRe.Match(raw) {
		t.Errorf("go.mod does not require %s v1.0.2", sdkModule)
	}
	if n := len(regexp.MustCompile(`(?m)^\s*replace\b`).FindAll(raw, -1)); n != 0 {
		t.Errorf("go.mod has %d replace directives, want 0", n)
	}
	toolRe := regexp.MustCompile(`(?m)^\s*tool\s+github\.com/golangci/golangci-lint/v2/cmd/golangci-lint\s*$`)
	if !toolRe.Match(raw) {
		t.Errorf("go.mod does not pin golangci-lint with a tool directive")
	}
	if _, err := os.Stat(filepath.Join(root, "third_party")); !os.IsNotExist(err) {
		t.Errorf("third_party must not exist (stat err: %v)", err)
	}
}

func TestLayoutFollowsADR0016(t *testing.T) {
	root := serviceRoot(t)
	internalDir := filepath.Join(root, "internal")

	for _, name := range []string{"handler", "service", "repository", "interceptor", "grpcserver", "bootstrap", "observability"} {
		if _, err := os.Stat(filepath.Join(internalDir, name)); !os.IsNotExist(err) {
			t.Errorf("internal/%s must not exist (stat err: %v)", name, err)
		}
	}
	for _, name := range []string{"app", "config", "rpc", "identity", "store", "events", "upstream", "token", "authz"} {
		info, err := os.Stat(filepath.Join(internalDir, name))
		if err != nil || !info.IsDir() {
			t.Errorf("internal/%s must exist as a directory (err: %v)", name, err)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "cmd", "server", "main.go")); err != nil || info.IsDir() {
		t.Errorf("cmd/server/main.go must exist (err: %v)", err)
	}

	domainDir := filepath.Join(internalDir, "identity")
	fset := token.NewFileSet()
	_ = filepath.WalkDir(domainDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("parse %s: %v", path, perr)
			return nil
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p == "google.golang.org/grpc" || strings.HasPrefix(p, "google.golang.org/grpc/") {
				t.Errorf("%s imports %s; internal/identity must stay transport-free", path, p)
			}
		}
		return nil
	})
}

func TestMakefileUsesSharedFiles(t *testing.T) {
	root := serviceRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	mk := string(raw)
	if !regexp.MustCompile(`(?m)^include\s+\.\./platform-core/make/go-service\.mk\s*$`).MatchString(mk) {
		t.Errorf("Makefile does not include ../platform-core/make/go-service.mk")
	}
	if !regexp.MustCompile(`(?m)^LINT_CONFIG\s*:=\s*\.\./platform-core/packages/go-sdk/\.golangci\.yml\s*$`).MatchString(mk) {
		t.Errorf("Makefile does not set LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml")
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == "generated" || n == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if n := d.Name(); n == ".golangci.yml" || n == ".golangci.yaml" || n == "go-service.mk" {
			t.Errorf("copy of shared file must not exist: %s", path)
		}
		return nil
	})
}
