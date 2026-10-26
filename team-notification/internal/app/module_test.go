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
	sdkModule  = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"
	sdkVersion = "v1.0.2"
)

func checkModule(gomod string) error {
	requireRe := regexp.MustCompile(`^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+(\S+)`)
	found := false
	for _, line := range strings.Split(gomod, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "replace ") || trimmed == "replace (" {
			return fmt.Errorf("replace directive not allowed: %q", trimmed)
		}
		if m := requireRe.FindStringSubmatch(line); m != nil {
			if m[1] != sdkVersion {
				return fmt.Errorf("go-sdk must be %s: %q", sdkVersion, trimmed)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("go.mod does not require %s %s", sdkModule, sdkVersion)
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
	if err := checkModule(readFile(t, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "third_party")); err == nil {
		t.Error("third_party directory must not exist")
	}
}

func TestModuleCheckRejectsReplaceAndWrongVersion(t *testing.T) {
	good := "module x\n\nrequire " + sdkModule + " " + sdkVersion + "\n"
	if err := checkModule(good); err != nil {
		t.Fatalf("valid go.mod rejected: %v", err)
	}
	cases := map[string]string{
		"replace":       good + "\nreplace " + sdkModule + " => ../x\n",
		"replace block": good + "\nreplace (\n\t" + sdkModule + " => ../x\n)\n",
		"wrong version": "module x\n\nrequire " + sdkModule + " v1.0.1\n",
		"block version": "module x\n\nrequire (\n\t" + sdkModule + " v0.9.0\n)\n",
		"missing":       "module x\n",
	}
	for name, in := range cases {
		err := checkModule(in)
		if err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	err := checkModule(good + "replace " + sdkModule + " => ../x\n")
	if err == nil || !strings.Contains(err.Error(), "replace") {
		t.Errorf("error must name the offending line, got %v", err)
	}
}

func TestMakefileIncludesSharedGoServiceMk(t *testing.T) {
	mk := readFile(t, "Makefile")
	for _, want := range []string{
		"SERVICE := team-notification",
		"GRPC_PORT := 50058",
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
