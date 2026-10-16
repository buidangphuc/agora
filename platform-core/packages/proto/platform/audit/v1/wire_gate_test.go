package auditv1_test

// Tests for change q6-trust-contract, tasks 1.6 and 2.3 (wire-compat gate and
// generation). Runs the real buf CLI; no network beyond the local git repo.
// Base ref defaults to "main" (override with Q6_BASE_REF).
//
// The generated-symbol test reads platform-core/gen, which is produced by
// `make -C platform-core proto`. It fails until generation has run.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const protoRel = "platform-core/packages/proto"

func run(t *testing.T, dir string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := run(t, ".", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("git toplevel: %v\n%s", err, out)
	}
	return strings.TrimSpace(out)
}

func baseInput(t *testing.T, root string) string {
	t.Helper()
	ref := os.Getenv("Q6_BASE_REF")
	if ref == "" {
		ref = "main"
	}
	common, err := run(t, root, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatalf("git common dir: %v\n%s", err, common)
	}
	return strings.TrimSpace(common) + "#ref=" + ref + ",subdir=" + protoRel
}

// Scenario: Trust contract: Change is backward compatible
func TestQ6Gate_ChangeIsBackwardCompatible(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, protoRel)
	if out, err := run(t, dir, "buf", "lint"); err != nil {
		t.Errorf("buf lint failed: %v\n%s", err, out)
	}
	if out, err := run(t, dir, "buf", "breaking", "--against", baseInput(t, root)); err != nil {
		t.Errorf("buf breaking against base failed: %v\n%s", err, out)
	}
}

// Scenario: Trust contract: Wire-breaking edit is rejected
func TestQ6Gate_WireBreakingEditIsRejected(t *testing.T) {
	root := repoRoot(t)
	scratch := t.TempDir()
	if out, err := run(t, root, "cp", "-R", filepath.Join(root, protoRel)+"/.", scratch); err != nil {
		t.Fatalf("copy proto tree: %v\n%s", err, out)
	}
	f := filepath.Join(scratch, "platform/listing/v1/listing.proto")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	const from, to = "  string title = 2;", "  string title = 99;"
	if !strings.Contains(string(b), from) {
		t.Fatalf("fixture anchor %q not found in listing.proto", from)
	}
	if err := os.WriteFile(f, []byte(strings.Replace(string(b), from, to, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, scratch, "buf", "breaking", "--against", baseInput(t, root))
	if err == nil {
		t.Fatalf("buf breaking accepted a renumbered field; output:\n%s", out)
	}
	if !strings.Contains(out, "title") {
		t.Errorf("buf breaking failed but does not name the field; output:\n%s", out)
	}
}

// Scenario: Trust contract: New symbols in every generated language
func TestQ6Gate_NewSymbolsInEveryGeneratedLanguage(t *testing.T) {
	root := repoRoot(t)
	gen := filepath.Join(root, "platform-core", "gen")
	symbols := []string{"ModerateListing", "ListModerationQueue", "ScreenListing", "AccountAdminService",
		"UserStatusChanged", "CreateReport", "ListReports", "ResolveReport", "ListOrderDisputes",
		"ListDisputes", "DisputeResolved"}
	// Per language: a symbol is present when its name (or the snake_case /
	// lowerCamel form emitted by that generator) occurs in the output tree.
	langs := map[string]func(string) []string{
		"go":     func(s string) []string { return []string{s} },
		"jvm":    func(s string) []string { return []string{s} },
		"ts":     func(s string) []string { return []string{s, strings.ToLower(s[:1]) + s[1:]} },
		"python": func(s string) []string { return []string{s, snake(s)} },
	}
	for lang, forms := range langs {
		var corpus strings.Builder
		n := 0
		_ = filepath.WalkDir(filepath.Join(gen, lang), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if b, e := os.ReadFile(p); e == nil {
				corpus.Write(b)
				n++
			}
			return nil
		})
		if n == 0 {
			t.Errorf("%s: no generated files under platform-core/gen/%s (run make -C platform-core proto)", lang, lang)
			continue
		}
		all := corpus.String()
		for _, s := range symbols {
			found := false
			for _, f := range forms(s) {
				if strings.Contains(all, f) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s output lacks symbol %s", lang, s)
			}
		}
	}
}

func snake(s string) string {
	var o []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				o = append(o, '_')
			}
			c += 'a' - 'A'
		}
		o = append(o, c)
	}
	return string(o)
}
