package gosdk

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	wantModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"
	oldImport  = "github.com/buidangphuc/platform-core/"
)

func platformCore(t *testing.T, elem ...string) string {
	t.Helper()
	return filepath.Join(append([]string{"..", ".."}, elem...)...)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestModulePathIsFetchable(t *testing.T) {
	mod := readFile(t, "go.mod")
	first := strings.TrimSpace(strings.SplitN(mod, "\n", 2)[0])
	if first != "module "+wantModule {
		t.Errorf("go.mod module line = %q, want %q", first, "module "+wantModule)
	}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || p == "release_test.go" {
			return nil
		}
		if strings.Contains(readFile(t, p), oldImport) {
			t.Errorf("%s still references %s", p, oldImport)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReleaseVersionIsDocumented(t *testing.T) {
	heading := regexp.MustCompile(`(?m)^## \[?v\d+\.\d+\.\d+\]?`)
	cl := readFile(t, "CHANGELOG.md")
	h := heading.FindString(cl)
	if !regexp.MustCompile(`^## \[?v1\.0\.1\]?$`).MatchString(h) {
		t.Errorf("first release heading of CHANGELOG.md = %q, want \"## v1.0.1\"", h)
	}
	if !regexp.MustCompile(`(?m)^Version:\s*v1\.0\.1\s*$`).MatchString(readFile(t, "README.md")) {
		t.Errorf("README.md has no line \"Version: v1.0.1\"")
	}
}

func TestTemplateRequiresTagWithoutReplace(t *testing.T) {
	tpl := platformCore(t, "templates", "_service-go")
	mod := readFile(t, filepath.Join(tpl, "go.mod"))
	req := regexp.MustCompile(`(?m)^\s*(require\s+)?` + regexp.QuoteMeta(wantModule) + `\s+v\d+\.\d+\.\d+`)
	if !req.MatchString(mod) {
		t.Errorf("template go.mod has no require of %s at a tag", wantModule)
	}
	for _, line := range strings.Split(mod, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "//") {
			continue
		}
		if strings.Contains(l, "go-sdk") && strings.Contains(l, "=>") {
			t.Errorf("template go.mod replaces the SDK: %q", l)
		}
	}
	if exists(filepath.Join(tpl, "third_party")) {
		t.Errorf("template has a third_party directory")
	}
}

func TestVendoringToolingIsRemoved(t *testing.T) {
	for _, f := range []string{"sync-go-sdk.sh", "sync-go-sdk_test.sh", "check-go-sdk-vendor.sh", "check-go-sdk-vendor_test.sh"} {
		if exists(platformCore(t, "tools", f)) {
			t.Errorf("platform-core/tools/%s still exists", f)
		}
	}
	root := filepath.Join(platformCore(t), "..")
	for _, f := range []string{filepath.Join(".github", "workflows", "asdlc-gates.yml"), filepath.Join("scripts", "asdlc", "ci_local.sh")} {
		p := filepath.Join(root, f)
		if !exists(p) {
			continue
		}
		body := readFile(t, p)
		for _, n := range []string{"sync-go-sdk", "check-go-sdk-vendor"} {
			if strings.Contains(body, n) {
				t.Errorf("%s still references %s", f, n)
			}
		}
	}
}

func TestSharedLintConfigIsServiceNeutral(t *testing.T) {
	cfg := readFile(t, ".golangci.yml")
	if strings.Contains(cfg, "BEGIN service-local") || strings.Contains(cfg, "END service-local") {
		t.Errorf(".golangci.yml still has a service-local block")
	}
	var prefixes []string
	lines := strings.Split(cfg, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "local-prefixes:" {
			continue
		}
		for _, n := range lines[i+1:] {
			s := strings.TrimSpace(n)
			if !strings.HasPrefix(s, "- ") {
				break
			}
			prefixes = append(prefixes, strings.Trim(strings.TrimPrefix(s, "- "), `"'`))
		}
	}
	if len(prefixes) != 1 || prefixes[0] != "github.com/buidangphuc" {
		t.Errorf("local-prefixes = %v, want [github.com/buidangphuc]", prefixes)
	}
}

func lintRules(t *testing.T) []string {
	t.Helper()
	var rules []string
	inRules := false
	var cur []string
	flush := func() {
		if cur != nil {
			rules = append(rules, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(readFile(t, ".golangci.yml"), "\n") {
		s := strings.TrimSpace(line)
		if s == "rules:" {
			inRules = true
			continue
		}
		if !inRules {
			continue
		}
		if s != "" && !strings.HasPrefix(line, " ") {
			break
		}
		if strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasPrefix(s, "- ") && strings.HasPrefix(strings.TrimLeft(line, " "), "- ") && len(line)-len(strings.TrimLeft(line, " ")) <= 6 {
			flush()
			cur = []string{s}
			continue
		}
		if cur != nil && s != "" {
			cur = append(cur, s)
		}
	}
	flush()
	return rules
}

func TestServiceExclusionsArePathScoped(t *testing.T) {
	rules := lintRules(t)
	if len(rules) == 0 {
		t.Fatalf("no exclusion rules parsed from .golangci.yml")
	}
	pathRe := regexp.MustCompile(`(?m)^(?:- )?path:\s*["']?([^"'\n]+)`)
	for _, r := range rules {
		m := pathRe.FindStringSubmatch(r)
		names := strings.Contains(r, "team-")
		switch {
		case m == nil && names:
			t.Errorf("rule without path names a service:\n%s", r)
		case m != nil && names && !strings.HasPrefix(strings.TrimSpace(m[1]), "team-"):
			t.Errorf("rule names a service but path %q does not start with team-:\n%s", m[1], r)
		}
	}
}

func TestTemplateReferencesSharedMakeAndLint(t *testing.T) {
	tpl := platformCore(t, "templates", "_service-go")
	mk := readFile(t, filepath.Join(tpl, "Makefile"))
	if !regexp.MustCompile(`(?m)^-?include\s+\.\./platform-core/make/go-service\.mk\s*$`).MatchString(mk) {
		t.Errorf("template Makefile does not include ../platform-core/make/go-service.mk")
	}
	if !strings.Contains(mk, "../platform-core/packages/go-sdk/.golangci.yml") {
		t.Errorf("template Makefile does not pass ../platform-core/packages/go-sdk/.golangci.yml")
	}
	for _, f := range []string{".golangci.yml", "go-service.mk"} {
		if exists(filepath.Join(tpl, f)) {
			t.Errorf("template holds a copy of %s", f)
		}
	}
}
