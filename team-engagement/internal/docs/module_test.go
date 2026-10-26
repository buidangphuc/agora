package docs

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const sdkPath = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestModuleUsesTaggedSDK(t *testing.T) {
	// Scenario: Engagement SDK: Module requires the tag and nothing else
	mod := readFile(t, "../../go.mod")
	re := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkPath) + `\s+(\S+)`)
	m := re.FindStringSubmatch(mod)
	if m == nil {
		t.Fatalf("go.mod does not require %s", sdkPath)
	}
	if m[1] != "v1.0.2" {
		t.Fatalf("SDK version = %s, want v1.0.2", m[1])
	}
	for _, line := range strings.Split(mod, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "replace") {
			t.Fatalf("go.mod must not contain a replace directive: %q", line)
		}
	}
	if _, err := os.Stat("../../third_party"); !os.IsNotExist(err) {
		t.Fatalf("third_party directory must not exist (stat err = %v)", err)
	}
	if !strings.Contains(mod, "github.com/golangci/golangci-lint") || !strings.Contains(mod, "tool ") {
		t.Fatal("go.mod must pin golangci-lint as a tool")
	}
}

func TestMakefileUsesSharedFiles(t *testing.T) {
	// Scenario: Engagement SDK: Makefile uses the shared files
	mk := readFile(t, "../../Makefile")
	if !regexp.MustCompile(`(?m)^-?include\s+\.\./platform-core/make/go-service\.mk\s*$`).MatchString(mk) {
		t.Fatal("Makefile must include ../platform-core/make/go-service.mk")
	}
	if !regexp.MustCompile(`(?m)^LINT_CONFIG\s*[:?]?=\s*\.\./platform-core/packages/go-sdk/\.golangci\.yml\s*$`).MatchString(mk) {
		t.Fatal("Makefile must set LINT_CONFIG to ../platform-core/packages/go-sdk/.golangci.yml")
	}
	if _, err := os.Stat("../../.golangci.yml"); !os.IsNotExist(err) {
		t.Fatalf("service must not carry its own .golangci.yml (stat err = %v)", err)
	}
}
