package store_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFakesNotImportedByProductionCode(t *testing.T) {
	root := "../.."
	exists := exec.CommandContext(t.Context(), "go", "list", "./internal/store/storefake")
	exists.Dir = root
	if out, err := exists.CombinedOutput(); err != nil {
		t.Fatalf("storefake package missing: %v\n%s", err, out)
	}
	cmd := exec.CommandContext(t.Context(), "go", "list", "-e", "-deps", "./cmd/server")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/server: %v\n%s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasSuffix(dep, "internal/store/storefake") {
			t.Fatalf("production binary depends on %s", dep)
		}
	}
}
