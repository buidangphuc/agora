package auditv1_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Task 3.1-3.2: consumer proto copies of the six trust-contract source protos.
// Pairs: source relative path -> consumer repos that must hold a byte-identical copy.
var copyPairs = map[string][]string{
	"listing/v1/listing.proto":       {"team-domain", "team-gateway", "team-frontend", "team-ai", "team-search", "team-order"},
	"ai/v1/ai.proto":                 {"team-ai", "team-domain", "team-gateway", "team-frontend", "team-search", "team-analytics"},
	"identity/v1/identity.proto":     {"team-identity", "team-gateway", "team-frontend", "team-domain"},
	"engagement/v1/engagement.proto": {"team-engagement", "team-gateway", "team-frontend", "team-order", "team-analytics"},
	"engagement/v1/events.proto":     {"team-engagement", "team-analytics", "team-order"},
	"audit/v1/audit.proto": {"team-analytics", "team-audit", "team-domain", "team-engagement", "team-frontend",
		"team-gateway", "team-order", "team-payment", "team-promotion", "team-verification"},
}

var lintRepos = []string{"team-domain", "team-gateway", "team-frontend", "team-ai", "team-search", "team-order",
	"team-identity", "team-engagement", "team-analytics", "team-audit", "team-payment", "team-promotion", "team-verification"}

// workspace root (the worktree) from platform-core/packages/proto/platform/audit/v1.
func root(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs("../../../../../..")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// checkCopies returns one problem per drifted or missing (source, copy) pair.
func checkCopies(srcRoot, consumerRoot string) []string {
	var bad []string
	for rel, repos := range copyPairs {
		src, err := os.ReadFile(filepath.Join(srcRoot, "platform-core/packages/proto/platform", rel))
		if err != nil {
			bad = append(bad, "SOURCE MISSING "+rel)
			continue
		}
		for _, repo := range repos {
			p := filepath.Join(consumerRoot, repo, "proto/platform", rel)
			got, err := os.ReadFile(p)
			switch {
			case err != nil:
				bad = append(bad, "MISSING "+repo+"/"+rel)
			case !bytes.Equal(src, got):
				bad = append(bad, "DRIFT "+repo+"/"+rel)
			}
		}
	}
	sort.Strings(bad)
	return bad
}

// Scenario: Trust contract: Copies match the source
func TestTrustContract_CopiesMatchTheSource(t *testing.T) {
	r := root(t)
	if bad := checkCopies(r, r); len(bad) > 0 {
		t.Fatalf("%d copies differ from source:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// Scenario: Trust contract: Drifted or missing copy is rejected
func TestTrustContract_DriftedOrMissingCopyIsRejected(t *testing.T) {
	r := root(t)
	scratch := t.TempDir()
	for rel, repos := range copyPairs {
		src, err := os.ReadFile(filepath.Join(r, "platform-core/packages/proto/platform", rel))
		if err != nil {
			t.Fatalf("source %s unreadable: %v", rel, err)
		}
		for _, repo := range repos {
			p := filepath.Join(scratch, repo, "proto/platform", rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, src, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if bad := checkCopies(r, scratch); len(bad) != 0 {
		t.Fatalf("pristine scratch tree must pass, got %v", bad)
	}
	// changed byte
	drift := filepath.Join(scratch, "team-order/proto/platform/engagement/v1/events.proto")
	b, _ := os.ReadFile(drift)
	b[len(b)-1] ^= 0x01
	if err := os.WriteFile(drift, b, 0o644); err != nil {
		t.Fatal(err)
	}
	// deleted copy
	if err := os.Remove(filepath.Join(scratch, "team-audit/proto/platform/audit/v1/audit.proto")); err != nil {
		t.Fatal(err)
	}
	bad := checkCopies(r, scratch)
	want := []string{"DRIFT team-order/engagement/v1/events.proto", "MISSING team-audit/audit/v1/audit.proto"}
	for _, w := range want {
		found := false
		for _, g := range bad {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q in %v", w, bad)
		}
	}
	if len(bad) != 2 {
		t.Errorf("expected exactly 2 problems, got %v", bad)
	}
}

// Scenario: Trust contract: Consumer copies lint
func TestTrustContract_ConsumerCopiesLint(t *testing.T) {
	buf, err := exec.LookPath("buf")
	if err != nil {
		t.Fatalf("buf not on PATH: %v", err)
	}
	r := root(t)
	for _, repo := range lintRepos {
		dir := filepath.Join(r, repo, "proto")
		if _, err := os.Stat(filepath.Join(dir, "buf.yaml")); err != nil {
			continue
		}
		cmd := exec.Command(buf, "lint")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("buf lint failed in %s/proto: %v\n%s", repo, err, out)
		}
	}
}
