package store_test

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseTestsFailWithoutDockerWhenRequired(t *testing.T) {
	for _, gone := range []string{"../repository", "../../migrations"} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s still exists: the package moved to internal/store and migrations are embedded from internal/store/migrations", gone)
		}
	}

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "docker_required_test.go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, banned := range []string{".Skip(", ".Skipf(", ".SkipNow("} {
			if strings.Contains(string(body), banned) {
				t.Errorf("%s contains %q: a Docker-backed suite must fail, not skip", path, banned)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan test files: %v", err)
	}

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPostgresFlashHoldMigrationRoundTrip$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"GO_SDK_REQUIRE_DOCKER=1",
		"PROMOTION_TEST_DATABASE_URL=postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=2",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	text := out.String()
	if runErr == nil {
		t.Fatalf("TestPostgresFlashHoldMigrationRoundTrip passed without a reachable database:\n%s", text)
	}
	if strings.Contains(text, "--- SKIP") {
		t.Fatalf("TestPostgresFlashHoldMigrationRoundTrip was skipped instead of failing:\n%s", text)
	}
	if !strings.Contains(text, "--- FAIL: TestPostgresFlashHoldMigrationRoundTrip") {
		t.Fatalf("TestPostgresFlashHoldMigrationRoundTrip did not report FAIL:\n%s", text)
	}
}
