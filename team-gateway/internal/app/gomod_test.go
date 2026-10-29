package app

import (
	"os"
	"os/exec"
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
		t.Fatal(err)
	}
	return root
}

func TestGoModRequiresSDKTagWithoutReplace(t *testing.T) {
	root := serviceRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mod := string(raw)

	require := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+v1\.0\.2(?:\s|$)`)
	if !require.MatchString(mod) {
		t.Errorf("go.mod must require %s v1.0.2", sdkModule)
	}
	if regexp.MustCompile(`(?m)^\s*replace\b`).MatchString(mod) {
		t.Errorf("go.mod must have no replace directive")
	}
	if _, err := os.Stat(filepath.Join(root, "third_party")); err == nil {
		t.Errorf("team-gateway/third_party must not exist")
	}
}

func TestMakefileIncludesSharedTargets(t *testing.T) {
	root := serviceRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "include ../platform-core/make/go-service.mk") {
		t.Errorf("Makefile must include ../platform-core/make/go-service.mk")
	}

	cmd := exec.Command("make", "help")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make help failed: %v\n%s", err, out)
	}
	for _, target := range []string{"check", "lint", "test", "check-env", "docker"} {
		if !regexp.MustCompile(`(?m)^\s*` + target + `\s`).Match(out) {
			t.Errorf("make help must list target %q; output:\n%s", target, out)
		}
	}
}
