package app_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func goSources(t *testing.T, includeTests bool) map[string]string {
	t.Helper()
	root := serviceRoot(t)
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "generated" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || (!includeTests && strings.HasSuffix(p, "_test.go")) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNoRuntimeFallbacksInSource(t *testing.T) {
	// scenario: Boot uses the real database and broker or fails
	forbidden := []string{
		"InMemory", "NewNoop", "RequireDurableStorage", "DATABASE_ENABLED", "KAFKA_ENABLED",
		"running in mock mode", "SHUTDOWN_GRACE_SECONDS", "OpenOrLocal",
	}
	for rel, src := range goSources(t, false) {
		if strings.HasSuffix(rel, "_teststub.go") {
			continue
		}
		for _, f := range forbidden {
			if strings.Contains(src, f) {
				t.Errorf("%s contains runtime fallback marker %q", rel, f)
			}
		}
	}
	for _, dir := range []string{"bootstrap", "grpcserver", "handler", "interceptor", "repository", "service", "consumer", "upstream"} {
		if _, err := os.Stat(filepath.Join(serviceRoot(t), "internal", dir)); err == nil {
			t.Errorf("internal/%s still exists; the old layout must be deleted", dir)
		}
	}
	main := goSources(t, false)[filepath.Join("cmd", "server", "main.go")]
	if strings.Contains(main, "pgxpool") {
		t.Error("cmd/server/main.go opens a pool itself; it must only call app.Run and app.Migrate")
	}
	if !strings.Contains(main, "app.Run") || !strings.Contains(main, "app.Migrate") {
		t.Error("cmd/server/main.go must dispatch to app.Run and app.Migrate")
	}
}

func TestDockerRequiredNeverSkips(t *testing.T) {
	// scenario: Docker-backed tests never skip
	if os.Getenv("GO_SDK_REQUIRE_DOCKER") == "" {
		t.Fatal("GO_SDK_REQUIRE_DOCKER is not set: Docker-backed tests would skip silently; a skip is a gate failure")
	}
	for rel, src := range goSources(t, true) {
		if strings.HasSuffix(rel, "_test.go") && (strings.Contains(src, "."+"Skip(") || strings.Contains(src, "."+"SkipNow(") || strings.Contains(src, "."+"Skipf(")) {
			t.Errorf("%s skips a test; Docker-backed tests must fail, not skip", rel)
		}
	}
}
