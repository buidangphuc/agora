package inventory_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const baseRef = "f3bdb2f8e545596e2bf414e95aaa3bccee82fe70"

var testFunc = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)

func git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func oldTests(t *testing.T) map[string]string {
	t.Helper()
	files := strings.Fields(git(t, "ls-tree", "-r", "--full-tree", "--name-only", baseRef, "team-chat"))
	found := map[string]string{}
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, m := range testFunc.FindAllStringSubmatch(git(t, "show", baseRef+":"+f), -1) {
			found[m[1]] = f
		}
	}
	return found
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func d9Section(t *testing.T, design string) string {
	t.Helper()
	i := strings.Index(design, "## D9 Test inventory")
	if i < 0 {
		t.Fatal("design.md has no D9 Test inventory section")
	}
	rest := design[i+len("## D9 Test inventory"):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestEveryOldTestIsVerifiedByOrRecordedDeleted_TestInventory(t *testing.T) {
	root := strings.TrimSpace(git(t, "rev-parse", "--show-toplevel"))
	change := filepath.Join(root, "openspec", "changes", "chat-go-sdk-v1")
	old := oldTests(t)
	if len(old) == 0 {
		t.Fatal("no Test functions found at base ref")
	}

	var verified strings.Builder
	specs, _ := filepath.Glob(filepath.Join(change, "specs", "*", "spec.md"))
	if len(specs) == 0 {
		t.Fatal("no spec.md found")
	}
	for _, s := range specs {
		for _, line := range strings.Split(read(t, s), "\n") {
			if strings.Contains(line, "VERIFIED BY") {
				verified.WriteString(line + "\n")
			}
		}
	}
	d9 := d9Section(t, read(t, filepath.Join(change, "design.md")))

	var missing []string
	for name, file := range old {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		if re.MatchString(verified.String()) || re.MatchString(d9) {
			continue
		}
		missing = append(missing, name+" ("+file+")")
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("%d old tests neither in a VERIFIED BY nor in D9 Test inventory:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func TestD9InventoryNamesNoUnknownTest_TestInventory(t *testing.T) {
	root := strings.TrimSpace(git(t, "rev-parse", "--show-toplevel"))
	d9 := d9Section(t, read(t, filepath.Join(root, "openspec", "changes", "chat-go-sdk-v1", "design.md")))
	old := oldTests(t)
	cur, _ := exec.Command("git", "-C", root, "grep", "-hoE", "func (Test[A-Za-z0-9_]+)\\(", "--", "team-chat").Output()
	known := map[string]bool{}
	for n := range old {
		known[n] = true
	}
	for _, m := range testFunc.FindAllStringSubmatch(string(cur), -1) {
		known[m[1]] = true
	}
	var unknown []string
	for _, n := range regexp.MustCompile("`(Test[A-Za-z0-9_]+)`").FindAllStringSubmatch(d9, -1) {
		if !known[n[1]] {
			unknown = append(unknown, n[1])
		}
	}
	if len(unknown) > 0 {
		t.Fatalf("D9 names tests that exist neither at base nor now: %v", unknown)
	}
}
