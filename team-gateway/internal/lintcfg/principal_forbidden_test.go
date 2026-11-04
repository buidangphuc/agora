package lintcfg

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sharedConfig = "../../../platform-core/packages/go-sdk/.golangci.yml"

func runLint(t *testing.T, target string) string {
	t.Helper()
	cfg, err := filepath.Abs(sharedConfig)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "tool", "golangci-lint", "run", "--config", cfg, "--output.text.path=stdout", target)
	cmd.Dir = "../.."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
	return out.String()
}

func principalFindings(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "sloglint") && strings.Contains(l, "principal") {
			n++
		}
	}
	return n
}

func TestGWFP13a_LintRejectsRawPrincipalKey(t *testing.T) {
	dir, err := os.MkdirTemp("../", "lintprobe")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	src := "package probe\n\nimport \"log/slog\"\n\nfunc F(id string) {\n\tslog.Info(\"x\", \"principal\", id)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runLint(t, "./internal/"+filepath.Base(dir))
	if principalFindings(out) != 1 {
		t.Fatalf("want one sloglint forbidden-key finding for \"principal\", got output:\n%s", out)
	}
}

func TestGWFP13b_NoPrincipalFindingsInGateway(t *testing.T) {
	out := runLint(t, "./...")
	if n := principalFindings(out); n != 0 {
		t.Fatalf("want 0 principal forbidden-key findings, got %d:\n%s", n, out)
	}
}
