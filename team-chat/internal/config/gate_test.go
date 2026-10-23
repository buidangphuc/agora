package config_test

import (
	"os/exec"
	"testing"
)

func TestSharedLintIsCleanOnWholeService_Task171(t *testing.T) {
	cmd := exec.Command("go", "tool", "golangci-lint", "run", "--config", "../platform-core/packages/go-sdk/.golangci.yml", "./...")
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shared lint (make check) is not clean: %v\n%s", err, out)
	}
}
