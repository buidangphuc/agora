package docs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var enableAllowed = map[string]bool{"OTEL_ENABLED": true, "GRPC_REFLECTION_ENABLED": true}

func hasGoFiles(t *testing.T, dir string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func scanFallbacks(t *testing.T, root string) []string {
	t.Helper()
	var bad []string
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			src := string(b)
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			for _, s := range []string{"InMemory", "Noop", "RequireDurableStorage", "DATABASE_ENABLED", "KAFKA_ENABLED"} {
				if strings.Contains(src, s) {
					bad = append(bad, rel+": "+s)
				}
			}
			for _, tok := range strings.FieldsFunc(src, func(r rune) bool {
				return !(r == '_' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
			}) {
				if strings.HasSuffix(tok, "_ENABLED") && !enableAllowed[tok] && tok != "DATABASE_ENABLED" && tok != "KAFKA_ENABLED" {
					bad = append(bad, rel+": "+tok)
				}
			}
			if !strings.HasPrefix(rel, "internal/store/storetest/") && strings.Contains(src, "internal/store/storetest") {
				bad = append(bad, rel+": imports internal/store/storetest")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(bad)
	return bad
}

func TestLayoutFollowsADR0016(t *testing.T) {
	// Scenario: Engagement SDK: Old packages are gone
	for _, old := range []string{"bootstrap", "grpcserver", "observability", "interceptor", "handler", "service", "repository", "consumer"} {
		if hasGoFiles(t, filepath.Join("..", old)) {
			t.Errorf("internal/%s must not contain Go files", old)
		}
	}
	for _, want := range []string{"app", "config", "rpc", "engagement", "store", "events", "upstream"} {
		if !hasGoFiles(t, filepath.Join("..", want)) {
			t.Errorf("internal/%s must exist with Go files", want)
		}
	}
	if !hasGoFiles(t, filepath.Join("..", "..", "cmd", "server")) {
		t.Error("cmd/server must exist")
	}
}

func TestNoRuntimeFallbacks(t *testing.T) {
	// Scenario: Engagement SDK: No fallback code in the binary
	if bad := scanFallbacks(t, filepath.Join("..", "..")); len(bad) > 0 {
		t.Fatalf("fallback code found:\n%s", strings.Join(bad, "\n"))
	}
}

func TestNoRuntimeFallbacksDetectsViolation(t *testing.T) {
	// Scenario: Engagement SDK: A reintroduced enable flag fails the scan
	cases := map[string]struct{ path, src, want string }{
		"kafka flag":   {"internal/config/x.go", "package config\nconst k = \"KAFKA_ENABLED\"\n", "internal/config/x.go: KAFKA_ENABLED"},
		"in memory":    {"internal/app/y.go", "package app\nvar r = NewInMemoryRepository()\n", "internal/app/y.go: InMemory"},
		"other enable": {"cmd/server/z.go", "package main\nconst k = \"SWEEP_ENABLED\"\n", "cmd/server/z.go: SWEEP_ENABLED"},
		"storetest":    {"internal/rpc/w.go", "package rpc\nimport _ \"x/internal/store/storetest\"\n", "internal/rpc/w.go: imports internal/store/storetest"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			p := filepath.Join(root, filepath.FromSlash(c.path))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(c.src), 0o644); err != nil {
				t.Fatal(err)
			}
			bad := scanFallbacks(t, root)
			found := false
			for _, b := range bad {
				if b == c.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("scan = %v, want it to report %q", bad, c.want)
			}
		})
	}
	t.Run("allowed", func(t *testing.T) {
		root := t.TempDir()
		p := filepath.Join(root, "internal", "config", "ok.go")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		src := "package config\nconst a = \"OTEL_ENABLED\"\nconst b = \"GRPC_REFLECTION_ENABLED\"\n"
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if bad := scanFallbacks(t, root); len(bad) != 0 {
			t.Fatalf("allowed flags reported: %v", bad)
		}
	})
}
