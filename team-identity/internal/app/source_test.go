package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerBinaryExcludesFakes(t *testing.T) {
	// scenario: The server binary does not link the in-memory fakes
	cmd := exec.Command("go", "list", "-deps", "./cmd/server")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/server: %v\n%s", err, out)
	}
	var sawApp bool
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasSuffix(pkg, "storetest") {
			t.Errorf("cmd/server links %s; an in-memory fake must never be a runtime fallback", pkg)
		}
		if strings.HasSuffix(pkg, "team-identity/internal/app") {
			sawApp = true
		}
	}
	if !sawApp {
		t.Error("cmd/server does not depend on internal/app")
	}
}

func TestReadmeDescribesTheSDKRuntime(t *testing.T) {
	// task 1.9.3: README configuration table, migrate subcommand, deploy order, purge, reflection
	raw, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(raw)
	for _, want := range []string{
		"migrate up",
		"liveness",
		"GRPC_REFLECTION_ENABLED=true",
		"OUTBOX_PURGE_RETENTION",
		"7 days",
		"0019",
		"UPDATE identity_outbox_events SET status='pending', attempts=0, available_at=now(), locked_until=NULL, error=NULL WHERE status='failed'",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README does not mention %q", want)
		}
	}
	for _, gone := range []string{
		"KAFKA_ENABLED", "DATABASE_ENABLED", "OUTBOX_ENABLED", "SESSION_SWEEP_ENABLED",
		"OUTBOX_MAX_ATTEMPTS", "UPSTREAM_CALL_TIMEOUT_SECONDS", "internal/events/relayer.go", "gofmt -l .",
	} {
		if strings.Contains(readme, gone) {
			t.Errorf("README still mentions removed %q", gone)
		}
	}
}
