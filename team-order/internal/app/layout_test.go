package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	sdkModule  = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"
	sdkVersion = "v1.0.2"
	lintTool   = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
)

func readService(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func exists(rel string) bool {
	_, err := os.Stat(filepath.Join("..", "..", rel))
	return err == nil
}

func hasFiles(rel string) bool {
	found := false
	_ = filepath.WalkDir(filepath.Join("..", "..", rel), func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = true
		}
		return nil
	})
	return found
}

func prodSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, top := range []string{"cmd", "internal"} {
		root := filepath.Join("..", "..", top)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, "_teststub.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(filepath.Join("..", ".."), p)
			out[rel] = string(b)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestGoModRequiresTaggedSDK(t *testing.T) {
	// scenario: go.mod requires the tag and nothing local
	gomod := readService(t, "go.mod")
	requireRe := regexp.MustCompile(`^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+(\S+)`)
	requires, hasTool := 0, false
	for _, line := range strings.Split(gomod, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "replace ") || trim == "replace (" {
			t.Errorf("replace directive not allowed: %q", trim)
		}
		if m := requireRe.FindStringSubmatch(line); m != nil {
			requires++
			if m[1] != sdkVersion {
				t.Errorf("go-sdk must be %s, got %q", sdkVersion, trim)
			}
		}
		if strings.HasPrefix(trim, "tool "+lintTool) || trim == lintTool {
			hasTool = true
		}
	}
	if requires != 1 {
		t.Errorf("go.mod has %d requires of %s, want exactly 1", requires, sdkModule)
	}
	if !hasTool {
		t.Errorf("go.mod has no tool directive for %s", lintTool)
	}
	if exists("third_party") {
		t.Error("third_party must not exist")
	}
}

func TestSharedMakeAndLintAreReferenced(t *testing.T) {
	// scenario: Shared make and lint files are referenced without a local copy
	mk := readService(t, "Makefile")
	for _, want := range []string{
		"SERVICE := team-order",
		"GRPC_PORT := 50055",
		"BINARIES := server resync-commits",
		"LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml",
		"include ../platform-core/make/go-service.mk",
	} {
		if !strings.Contains(mk, want) {
			t.Errorf("Makefile missing %q", want)
		}
	}
	for _, cp := range []string{".golangci.yml", "make/go-service.mk", "go-service.mk", "third_party"} {
		if exists(cp) {
			t.Errorf("%s must not exist in team-order", cp)
		}
	}
}

func TestLayoutHasNoLegacyPackages(t *testing.T) {
	// scenario: Hand-rolled infrastructure packages are gone
	for _, dir := range []string{"bootstrap", "featureflags", "interceptor", "grpcserver", "handler", "service", "repository", "consumer"} {
		if hasFiles(filepath.Join("internal", dir)) {
			t.Errorf("internal/%s still has files; the legacy package must be deleted", dir)
		}
	}
	for _, dir := range []string{"app", "config", "rpc", "order", "store", "events", "upstream"} {
		if !hasFiles(filepath.Join("internal", dir)) {
			t.Errorf("internal/%s is missing", dir)
		}
	}
}

func TestNoRuntimeInMemoryFallback(t *testing.T) {
	// scenario: No runtime fallback exists in non-test code
	fakePrefix := filepath.Join("internal", "store", "fake") + string(filepath.Separator)
	for rel, src := range prodSources(t) {
		if strings.HasPrefix(rel, fakePrefix) {
			continue
		}
		for _, marker := range []string{
			"NewInMemory", "NoopPublisher", "RequireDurableStorage", "internal/store/fake",
			"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "FEATURE_FLAGS_ENABLED",
		} {
			if strings.Contains(src, marker) {
				t.Errorf("%s contains runtime fallback marker %q", rel, marker)
			}
		}
	}
}

func exportedIdents(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	fset := token.NewFileSet()
	entries, err := os.ReadDir(filepath.Join("..", "..", dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join("..", "..", dir, n), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					names = append(names, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							names = append(names, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, id := range s.Names {
							if id.IsExported() {
								names = append(names, id.Name)
							}
						}
					}
				}
			}
		}
	}
	return names
}

func TestNoDisputeStartDecision(t *testing.T) {
	// scenario: No start-decision function remains
	for _, dir := range []string{filepath.Join("internal", "app"), filepath.Join("internal", "events")} {
		for _, n := range exportedIdents(t, dir) {
			if n == "DisputeConsumerStartDecision" {
				t.Errorf("%s still exports DisputeConsumerStartDecision", dir)
			}
		}
	}
	for rel, src := range prodSources(t) {
		if strings.Contains(src, "DisputeConsumerStartDecision") {
			t.Errorf("%s still mentions DisputeConsumerStartDecision", rel)
		}
	}
}

func TestCompositionRootWiring(t *testing.T) {
	// scenario: A Kafka outage does not affect readiness (readiness is the single postgres check)
	var b strings.Builder
	for rel, src := range prodSources(t) {
		if strings.HasPrefix(rel, filepath.Join("internal", "app")+string(filepath.Separator)) {
			b.WriteString(src)
		}
	}
	s := b.String()
	if s == "" {
		t.Fatal("internal/app has no production source")
	}
	for _, want := range []string{"grpcx.NewServer", "lifecycle.Run", "lifecycle.StopAfter", "auditx.NewGate", "flags.New", `Register("postgres"`} {
		if !strings.Contains(s, want) {
			t.Errorf("internal/app does not use %s", want)
		}
	}
	if n := strings.Count(s, ".Register(\""); n != 1 {
		t.Errorf("internal/app registers %d health checks, want exactly 1 (postgres)", n)
	}
	for _, banned := range []string{"pkg/migrate", "migrate.Up", "grpc.NewServer(", "health.NewServer("} {
		if strings.Contains(s, banned) {
			t.Errorf("internal/app contains %q", banned)
		}
	}
}

func TestMainOnlyDispatches(t *testing.T) {
	// scenario: Boot never migrates
	main := prodSources(t)[filepath.Join("cmd", "server", "main.go")]
	if !strings.Contains(main, "app.Run") || !strings.Contains(main, "migrate.Up") {
		t.Error("cmd/server/main.go must call app.Run and migrate.Up")
	}
	if strings.Contains(main, "pgxpool") {
		t.Error("cmd/server/main.go opens a pool itself")
	}
}
