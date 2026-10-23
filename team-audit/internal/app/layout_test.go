package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const sdkModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"

func serviceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func absent(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err == nil {
		return false
	}
	if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return true
}

func TestModuleRequiresSDKTag(t *testing.T) {
	root := serviceRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	required := false
	inRequire, inReplace := false, false
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "require ("):
			inRequire = true
			continue
		case strings.HasPrefix(line, "replace ("):
			inReplace = true
			continue
		case line == ")":
			inRequire, inReplace = false, false
			continue
		}
		if strings.HasPrefix(line, "replace ") || inReplace {
			t.Errorf("go.mod must contain no replace directive: %q", line)
		}
		spec := line
		if strings.HasPrefix(line, "require ") {
			spec = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		} else if !inRequire {
			continue
		}
		f := strings.Fields(spec)
		if len(f) >= 2 && f[0] == sdkModule && f[1] == "v1.0.2" {
			required = true
		}
	}
	if !required {
		t.Errorf("go.mod must require %s v1.0.2", sdkModule)
	}
	if !absent(t, filepath.Join(root, "third_party")) {
		t.Error("third_party must not exist")
	}
}

func TestLayoutFollowsADR0016(t *testing.T) {
	root := serviceRoot(t)
	for _, legacy := range []string{"handler", "service", "repository", "interceptor", "grpcserver"} {
		if !absent(t, filepath.Join(root, "internal", legacy)) {
			t.Errorf("internal/%s must not exist", legacy)
		}
	}
	dir := filepath.Join(root, "internal", "audit")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("internal/audit must exist: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p == "google.golang.org/grpc" || strings.HasPrefix(p, "google.golang.org/grpc/") ||
				strings.Contains(p, "/generated/") || strings.HasSuffix(p, "/generated") {
				t.Errorf("internal/audit/%s imports transport package %s", e.Name(), p)
			}
		}
	}
}

func TestMakefileUsesSharedFiles(t *testing.T) {
	root := serviceRoot(t)
	mk, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"SERVICE := team-audit",
		"GRPC_PORT := 50066",
		"LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml",
		"include ../platform-core/make/go-service.mk",
	} {
		if !strings.Contains(string(mk), want) {
			t.Errorf("Makefile must contain %q", want)
		}
	}
	for _, name := range []string{".golangci.yml", "go-service.mk"} {
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && (info.Name() == "generated" || info.Name() == ".git") {
				return filepath.SkipDir
			}
			if !info.IsDir() && info.Name() == name {
				t.Errorf("%s must not exist under team-audit", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
