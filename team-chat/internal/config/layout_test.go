package config

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestGoModRequiresTagWithoutReplace(t *testing.T) {
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
	if !strings.Contains(string(b), "tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint") {
		t.Error("go.mod must declare the golangci-lint tool")
	}
	if _, err := os.Lstat(filepath.Join(root, "third_party")); !os.IsNotExist(err) {
		t.Error("third_party must not exist")
	}
	df, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(df), "third_party") {
		t.Error("Dockerfile must not reference third_party")
	}
	if !strings.Contains(string(df), "./cmd/server") {
		t.Error("Dockerfile must keep building ./cmd/server")
	}
}

func TestSharedMakeAndLintAreReferenced(t *testing.T) {
	root := serviceRoot(t)
	mk, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("Makefile must exist: %v", err)
	}
	for _, want := range []string{
		"SERVICE := team-chat",
		"GRPC_PORT := 50057",
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
				t.Errorf("%s must not exist under team-chat", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMainOnlyCallsAppRun(t *testing.T) {
	// scenario: main stays a thin entry point
	b, err := os.ReadFile(filepath.Join(serviceRoot(t), "cmd", "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{"config.Load", "app.Run", "app.Migrate", "signal.NotifyContext"} {
		if !strings.Contains(src, want) {
			t.Errorf("main.go lacks %s", want)
		}
	}
	for _, bad := range []string{
		"internal/bootstrap", "internal/observability", "internal/grpcserver", "internal/handler",
		"internal/repository", "internal/service", "internal/events", "internal/store", "internal/rpc",
		"pgxpool", "kafkax", "go-sdk/pkg/outbox", "google.golang.org/grpc\"",
	} {
		if strings.Contains(src, bad) {
			t.Errorf("main.go references %s", bad)
		}
	}
	if n := strings.Count(src, "\n"); n > 45 {
		t.Errorf("main.go has %d lines, want the short ADR-0016 shape", n)
	}
	for _, gone := range []string{"bootstrap", "observability", "grpcserver"} {
		if _, err := os.Stat(filepath.Join(serviceRoot(t), "internal", gone)); !os.IsNotExist(err) {
			t.Errorf("internal/%s must be deleted", gone)
		}
	}
}
