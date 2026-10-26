package readmedoc

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func section(t *testing.T, md, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^## ` + regexp.QuoteMeta(name) + `\s*$`)
	loc := re.FindStringIndex(md)
	if loc == nil {
		t.Fatalf("README has no %q section", name)
	}
	rest := md[loc[1]:]
	if n := regexp.MustCompile(`(?m)^## `).FindStringIndex(rest); n != nil {
		rest = rest[:n[0]]
	}
	return rest
}

func TestReadmeConfigTableMatchesEnvExample_TheEnvExampleMatchesTheSettings(t *testing.T) {
	cfg := section(t, read(t, "README.md"), "Configuration")
	n := 0
	for _, line := range strings.Split(read(t, ".env.example"), "\n") {
		k, _, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		n++
		if !strings.Contains(cfg, "`"+k+"`") {
			t.Errorf("Configuration section does not list %s", k)
		}
	}
	if n == 0 {
		t.Error(".env.example has no variables")
	}
	for _, r := range []string{"DATABASE_URL", "ENV", "GRPC_PORT"} {
		ok := false
		for _, line := range strings.Split(cfg, "\n") {
			if strings.Contains(line, "`"+r+"`") && strings.Contains(strings.ToLower(line), "required") {
				ok = true
			}
		}
		if !ok {
			t.Errorf("Configuration must state %s is required on its row", r)
		}
	}
}

func TestReadmeDropsStaleClaims_TheEnvExampleMatchesTheSettings(t *testing.T) {
	md := read(t, "README.md")
	for i, line := range strings.Split(md, "\n") {
		l := strings.ToLower(line)
		if strings.Contains(l, "removed") {
			continue
		}
		for _, bad := range []string{"in-memory fallback", "inert", "no graceful drain", "not read", "shutdown_grace_seconds", "database_enabled", "drift gate"} {
			if strings.Contains(l, bad) {
				t.Errorf("README.md:%d mentions %q", i+1, bad)
			}
		}
	}
}

func TestReadmeLayoutAndSDKTag_TheEnvExampleMatchesTheSettings(t *testing.T) {
	md := read(t, "README.md")
	for _, p := range []string{"internal/app", "internal/rpc", "internal/referral", "internal/store", "ADR-0016", "go-sdk", "v1.0.2"} {
		if !strings.Contains(md, p) {
			t.Errorf("README missing %s", p)
		}
	}
	for _, p := range []string{"internal/handler", "internal/service", "internal/interceptor", "internal/repository", "internal/config/config.go` reads only"} {
		if strings.Contains(md, p) {
			t.Errorf("README still mentions old layout %s", p)
		}
	}
}

func TestReadmeBuildTableUsesMakeCheck_TheEnvExampleMatchesTheSettings(t *testing.T) {
	b := section(t, read(t, "README.md"), "Build, test, lint")
	if !strings.Contains(b, "make check") {
		t.Error("Build table missing make check")
	}
}

func TestReadmeKeepsContractEventsData_TheEnvExampleMatchesTheSettings(t *testing.T) {
	md := read(t, "README.md")
	for _, s := range []string{"Contract", "Events", "Data"} {
		section(t, md, s)
	}
	if !strings.Contains(section(t, md, "Events"), "None") {
		t.Error("Events section must say None")
	}
}
