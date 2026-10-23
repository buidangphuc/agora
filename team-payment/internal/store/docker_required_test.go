package store_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerRequiredFailsInsteadOfSkipping(t *testing.T) {
	for _, gone := range []string{"../pgtest", "../repository"} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s still exists: Postgres tests must use the SDK testkit (GO_SDK_REQUIRE_DOCKER makes a missing daemon fail), and the package moved to internal/store", gone)
		}
	}

	var usesTestkit bool
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
		src := string(body)
		for _, banned := range []string{".Skip(", ".Skipf(", ".SkipNow(", "TEST_DATABASE_URL", "internal/pgtest"} {
			if strings.Contains(src, banned) {
				t.Errorf("%s contains %q: a Docker-backed suite must fail, not skip, without a daemon", path, banned)
			}
		}
		if strings.Contains(src, "go-sdk/pkg/testkit") {
			usesTestkit = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan test files: %v", err)
	}
	if !usesTestkit {
		t.Error("no store test imports the SDK testkit for Postgres")
	}
}
