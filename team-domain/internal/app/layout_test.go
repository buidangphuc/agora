package app_test

import (
	"os"
	"path/filepath"
	"regexp"
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

func TestGoModRequiresTaggedSDK(t *testing.T) {
	root := serviceRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	requireRe := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+v1\.0\.2\s*(?://.*)?$`)
	if !requireRe.Match(raw) {
		t.Errorf("go.mod does not require %s v1.0.2", sdkModule)
	}

	replaceRe := regexp.MustCompile(`(?m)^\s*replace\b`)
	if n := len(replaceRe.FindAll(raw, -1)); n != 0 {
		t.Errorf("go.mod has %d replace directives, want 0", n)
	}

	toolRe := regexp.MustCompile(`(?m)^\s*(?:tool\s+)?github\.com/golangci/golangci-lint/v2/cmd/golangci-lint\s*$`)
	if !strings.Contains(string(raw), "tool") || !toolRe.Match(raw) {
		t.Errorf("go.mod does not declare golangci-lint as a tool")
	}

	if _, err := os.Stat(filepath.Join(root, "third_party")); !os.IsNotExist(err) {
		t.Errorf("team-domain/third_party must not exist (stat err: %v)", err)
	}
}

func TestLegacyPackagesRemoved(t *testing.T) {
	internalDir := filepath.Join(serviceRoot(t), "internal")

	gone := []string{
		"bootstrap", "grpcserver", "interceptor", "observability", "handler",
		"service", "repository", "consumer", "upstream", "storage",
	}
	for _, name := range gone {
		if _, err := os.Stat(filepath.Join(internalDir, name)); !os.IsNotExist(err) {
			t.Errorf("internal/%s must not exist (stat err: %v)", name, err)
		}
	}

	present := []string{"app", "config", "rpc", "listing", "store", "events", "objects", "screen", "audit"}
	for _, name := range present {
		info, err := os.Stat(filepath.Join(internalDir, name))
		if err != nil || !info.IsDir() {
			t.Errorf("internal/%s must exist as a directory (err: %v)", name, err)
		}
	}
}
