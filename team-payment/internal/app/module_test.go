package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	sdkModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"
	lintTool  = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
)

var sdkTagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

func checkModule(gomod string) error {
	requireRe := regexp.MustCompile(`^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+(\S+)`)
	requires, hasTool := 0, false
	for _, line := range strings.Split(gomod, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "replace ") || trimmed == "replace (" {
			return fmt.Errorf("replace directive not allowed: %q", trimmed)
		}
		if m := requireRe.FindStringSubmatch(line); m != nil {
			if !sdkTagRe.MatchString(m[1]) {
				return fmt.Errorf("go-sdk must be a tagged release vX.Y.Z: %q", trimmed)
			}
			requires++
		}
		if trimmed == "tool "+lintTool || trimmed == lintTool {
			hasTool = true
		}
	}
	if requires != 1 {
		return fmt.Errorf("go.mod has %d requires of %s, want exactly 1", requires, sdkModule)
	}
	if !hasTool {
		return fmt.Errorf("go.mod has no tool directive for %s", lintTool)
	}
	return nil
}

func readFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func TestModuleRequiresSDKTagWithoutReplace(t *testing.T) {
	// scenario: go.mod requires the tag and nothing local
	if err := checkModule(readFile(t, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "third_party")); err == nil {
		t.Error("third_party directory must not exist")
	}
}

func TestModuleCheckRejectsReplaceAndWrongVersion(t *testing.T) {
	// scenario: A replace directive or a copied shared file fails the layout checks
	good := "module x\n\nrequire " + sdkModule + " v1.0.3\n\ntool " + lintTool + "\n"
	if err := checkModule(good); err != nil {
		t.Fatalf("valid go.mod rejected: %v", err)
	}
	cases := map[string]string{
		"replace":        good + "\nreplace " + sdkModule + " => ../x\n",
		"replace block":  good + "\nreplace (\n\t" + sdkModule + " => ../x\n)\n",
		"pseudo-version": "module x\n\nrequire " + sdkModule + " v1.0.4-0.20261105000000-abcdef123456\n\ntool " + lintTool + "\n",
		"pre-release":    "module x\n\nrequire (\n\t" + sdkModule + " v1.0.4-rc.1\n)\n\ntool " + lintTool + "\n",
		"two requires":   good + "\nrequire " + sdkModule + " v1.0.2\n",
		"no lint tool":   "module x\n\nrequire " + sdkModule + " v1.0.3\n",
		"missing":        "module x\n",
	}
	for name, in := range cases {
		if err := checkModule(in); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	err := checkModule(good + "replace " + sdkModule + " => ../x\n")
	if err == nil || !strings.Contains(err.Error(), "replace") {
		t.Errorf("error must name the offending line, got %v", err)
	}
}

func TestMakefileIncludesSharedGoServiceMk(t *testing.T) {
	// scenario: Shared make and lint files are referenced, not copied
	mk := readFile(t, "Makefile")
	for _, want := range []string{
		"SERVICE := team-payment",
		"GRPC_PORT := 50056",
		"LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml",
		"include ../platform-core/make/go-service.mk",
	} {
		if !strings.Contains(mk, want) {
			t.Errorf("Makefile missing %q", want)
		}
	}
	for _, cp := range []string{".golangci.yml", "go-service.mk"} {
		if _, err := os.Stat(filepath.Join("..", "..", cp)); err == nil {
			t.Errorf("%s copy must not exist in the service", cp)
		}
	}
}
