package gosdk

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func firstReleaseSection(t *testing.T) string {
	t.Helper()
	cl := readFile(t, "CHANGELOG.md")
	loc := regexp.MustCompile(`(?m)^## `).FindAllStringIndex(cl, 2)
	if len(loc) == 0 {
		t.Fatalf("CHANGELOG.md has no release heading")
	}
	if len(loc) == 1 {
		return cl[loc[0][0]:]
	}
	return cl[loc[0][0]:loc[1][0]]
}

func requireAll(t *testing.T, name, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s does not mention %q", name, w)
		}
	}
}

func TestReleaseNotesDescribeGoalsG1ToG6(t *testing.T) {
	sec := readFile(t, "CHANGELOG.md")
	requireAll(t, "CHANGELOG", sec,
		"mockdata", "auditx", "key_fp", "AlreadyExists", "23505", "enable flag")
}

func TestReadmeDocumentsRelease(t *testing.T) {
	rd := readFile(t, "README.md")
	requireAll(t, "README.md", rd,
		"go get "+wantModule+"@v1.0.2",
		"mockdata", "auditx",
		"MOCK_DATA", "UPSTREAM_AUDIT_ADDR", "LOG_KEY_FP_SECRET",
		"enable flag",
		"go-service.mk", ".golangci.yml")
	if !regexp.MustCompile(`(?i)(never|do not|don't|not)[^\n]*(format|interpolat)[^\n]*(id|error)|(id|error)[^\n]*(never|do not|not)[^\n]*(format|interpolat)`).MatchString(rd) {
		t.Errorf("README.md has no rule against formatting ids into errors")
	}
	for _, stale := range []string{"sync-go-sdk", "check-go-sdk-vendor", "third_party/go-sdk"} {
		for i, line := range strings.Split(rd, "\n") {
			l := strings.ToLower(line)
			if strings.Contains(line, stale) && !strings.Contains(l, "removed") && !strings.Contains(l, "no longer") && !strings.Contains(l, "no ") {
				t.Errorf("README.md line %d still documents %s: %q", i+1, stale, line)
			}
		}
	}
	if regexp.MustCompile(`(?m)^#+\s.*\b[Ss]ync`).MatchString(rd) {
		t.Errorf("README.md still has a sync section")
	}
}

func adrFiles(t *testing.T) (adr16, adr18 string) {
	t.Helper()
	dir := platformCore(t, "docs", "ADR")
	m16, _ := filepath.Glob(filepath.Join(dir, "0016-*.md"))
	m18, _ := filepath.Glob(filepath.Join(dir, "0018-*.md"))
	if len(m16) != 1 {
		t.Fatalf("want one ADR-0016 file, got %v", m16)
	}
	if len(m18) != 1 {
		t.Fatalf("want one ADR-0018 file, got %v", m18)
	}
	return readFile(t, m16[0]), readFile(t, m18[0])
}

func TestADR0016IsSupersededInPartByADR0018(t *testing.T) {
	a16, _ := adrFiles(t)
	status := ""
	for _, l := range strings.Split(a16, "\n") {
		if strings.HasPrefix(l, "**Status:**") {
			status = l
			break
		}
	}
	if !strings.Contains(status, "Superseded in part by ADR-0018") {
		t.Errorf("ADR-0016 status line = %q, want it to contain \"Superseded in part by ADR-0018\"", status)
	}
}

func TestADR0018RecordsTagDistribution(t *testing.T) {
	_, a18 := adrFiles(t)
	if !strings.Contains(a18, "**Status:**") {
		t.Errorf("ADR-0018 has no Status line")
	}
	requireAll(t, "ADR-0018", a18,
		"ADR-0016", "Supersede", "sync-go-sdk.sh", "third_party", "replace",
		"platform-core/packages/go-sdk/vX.Y.Z", wantModule,
		"enable flag", "go-service.mk", ".golangci.yml", "mock data", "mock service")
}
