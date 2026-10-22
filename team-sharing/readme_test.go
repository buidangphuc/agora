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

func TestReadmeNoRemovedConfigOutsideRemovedNotes_AlwaysUsesPostgresNoInMemoryMode(t *testing.T) {
	md := read(t, "README.md")
	for i, line := range strings.Split(md, "\n") {
		if strings.Contains(strings.ToLower(line), "removed") {
			continue
		}
		for _, bad := range []string{"DATABASE_ENABLED", "in-memory", "SHUTDOWN_GRACE_SECONDS"} {
			if strings.Contains(line, bad) {
				t.Errorf("README.md:%d mentions %q outside a removed note", i+1, bad)
			}
		}
	}
}

func TestReadmeLayoutUsesAdr0016Packages_AlwaysUsesPostgresNoInMemoryMode(t *testing.T) {
	md := read(t, "README.md")
	for _, p := range []string{"internal/app", "internal/rpc", "internal/sharelink", "internal/store"} {
		if !strings.Contains(md, p) {
			t.Errorf("README layout missing %s", p)
		}
	}
	for _, p := range []string{"internal/grpcserver", "internal/interceptor", "internal/repository"} {
		if strings.Contains(md, p) {
			t.Errorf("README still mentions old package %s", p)
		}
	}
}

func TestReadmeConfigTableMatchesEnvExample_SettingsAreSDKGroupsAndEnvExampleMatches(t *testing.T) {
	cfg := section(t, read(t, "README.md"), "Configuration")
	env := read(t, ".env.example")
	n := 0
	for _, line := range strings.Split(env, "\n") {
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

func TestReadmeHealthSection_AlwaysUsesPostgresNoInMemoryMode(t *testing.T) {
	h := section(t, read(t, "README.md"), "Health")
	for _, w := range []string{"NOT_SERVING", "database", "liveness"} {
		if !strings.Contains(h, w) {
			t.Errorf("Health section missing %q", w)
		}
	}
}

func TestReadmeRunLocallyUsesMakeCheck_SettingsAreSDKGroupsAndEnvExampleMatches(t *testing.T) {
	r := section(t, read(t, "README.md"), "Run locally")
	if !strings.Contains(r, "make check") {
		t.Error("Run locally missing make check")
	}
}

func TestReadmeKnownGapsPruned_AlwaysUsesPostgresNoInMemoryMode(t *testing.T) {
	g := strings.ToLower(section(t, read(t, "README.md"), "Known gaps"))
	for _, bad := range []string{"fallback", "drift", "always serving", "not read by the code", "unread"} {
		if strings.Contains(g, bad) {
			t.Errorf("Known gaps still lists stale gap %q", bad)
		}
	}
	for _, keep := range []string{"created_by", "rate limit"} {
		if !strings.Contains(g, keep) {
			t.Errorf("Known gaps must keep %q", keep)
		}
	}
}
