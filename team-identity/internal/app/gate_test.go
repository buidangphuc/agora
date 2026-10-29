package app_test

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func runIn(t *testing.T, dir string, name string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err == nil {
		return out.String(), 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.ExitCode()
	}
	t.Fatalf("run %s: %v", name, err)
	return "", -1
}

func TestGateNoLegacyTokensRemain_1_10_1(t *testing.T) {
	root := serviceRoot(t)
	out, code := runIn(t, root, "git", "grep", "-n",
		"-e", "DATABASE_ENABLED", "-e", "KAFKA_ENABLED", "-e", "OUTBOX_ENABLED",
		"-e", "SESSION_SWEEP_ENABLED", "-e", "NoopPublisher", "-e", "OpenOrLocal",
		"-e", "internal/service", "-e", "internal/repository", "-e", "internal/handler",
		"--", ".",
		":!internal/config/*_test.go", ":!README.md", ":!*_test.go")
	if code > 1 {
		t.Fatalf("git grep failed (%d): %s", code, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("legacy tokens still present in tracked files:\n%s", out)
	}
}

func TestGateReadmeHasNoKafkaEnabled_1_10_1(t *testing.T) {
	root := serviceRoot(t)
	out, code := runIn(t, root, "git", "grep", "-n", "KAFKA_ENABLED", "--", "README.md")
	if code > 1 {
		t.Fatalf("git grep failed (%d): %s", code, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("README still mentions KAFKA_ENABLED:\n%s", out)
	}
}

func TestGateReadmeRefreshDocsScript_1_10_1(t *testing.T) {
	root := serviceRoot(t)
	if out, code := runIn(t, root, "sh", "tests/readme_refresh_docs_test.sh"); code != 0 {
		t.Errorf("readme_refresh_docs_test.sh exit %d:\n%s", code, out)
	}
}

func TestGateServerBinaryExcludesStoretest_1_10_1(t *testing.T) {
	root := serviceRoot(t)
	out, code := runIn(t, root, "go", "list", "-deps", "./cmd/server")
	if code != 0 {
		t.Fatalf("go list failed (%d): %s", code, out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "storetest") {
			t.Errorf("cmd/server depends on test-only package %s", l)
		}
	}
}
