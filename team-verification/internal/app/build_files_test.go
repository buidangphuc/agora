package app

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const sdkModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"

var sdkTagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

func serviceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func goModLines(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(serviceRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

func TestGoModRequiresSDKTagWithoutReplace(t *testing.T) {
	requires := 0
	inRequire, inReplace := false, false
	for _, raw := range goModLines(t) {
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
		if len(f) >= 2 && f[0] == sdkModule {
			requires++
			if !sdkTagRe.MatchString(f[1]) {
				t.Errorf("go-sdk must be a tagged release vX.Y.Z, got %q", f[1])
			}
		}
	}
	if requires != 1 {
		t.Errorf("go.mod has %d requires of %s, want exactly 1", requires, sdkModule)
	}
}

func TestNoVendoredSDKOrCopiedBuildFiles(t *testing.T) {
	root := serviceRoot(t)
	for _, p := range []string{"third_party", "make", ".golangci.yml"} {
		if _, err := os.Lstat(filepath.Join(root, p)); err == nil {
			t.Errorf("%s must not exist", p)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	mk, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(mk)
	if !strings.Contains(s, "include ../platform-core/make/go-service.mk") {
		t.Error("Makefile must include ../platform-core/make/go-service.mk")
	}
	if !strings.Contains(s, "LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml") {
		t.Error("Makefile must set LINT_CONFIG to the SDK golangci config")
	}
}

func TestGoModDeclaresLintTool(t *testing.T) {
	found := false
	for _, raw := range goModLines(t) {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "tool ") && strings.Contains(line, "github.com/golangci/golangci-lint/v2/cmd/golangci-lint") {
			found = true
		}
	}
	if !found {
		t.Error("go.mod must declare the golangci-lint tool")
	}
}

func TestMakefileAndDockerfileUseSharedTemplate(t *testing.T) {
	root := serviceRoot(t)
	mk, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SERVICE := team-verification", "GRPC_PORT := 50064"} {
		if !strings.Contains(string(mk), want) {
			t.Errorf("Makefile must contain %q", want)
		}
	}
	df, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(df), "50064") {
		t.Error("Dockerfile must use GRPC_PORT 50064")
	}
}
