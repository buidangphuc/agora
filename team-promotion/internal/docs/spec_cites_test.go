package docs

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const thisChange = "promotion-go-sdk-v1"

var (
	pairRe = regexp.MustCompile("internal/([a-z]+)` `(Test[A-Za-z0-9_]+)")
	funcRe = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)

	replacedNames = map[string]bool{
		"TestDeclaredEnvKeysNoDrift":                    true,
		"TestAuditAddrDeclaredEnvKey":                   true,
		"TestNewAuditClientEmptyAddrNil":                true,
		"TestCreateVoucherNilAuditUnavailable":          true,
		"TestCreateVoucherShopAdminNilAuditUnavailable": true,
		"TestCreateVoucherPlatformSellerDeniedNilAudit": true,
	}

	removedPackages = []string{"service", "handler", "repository", "producer", "interceptor", "grpcserver", "bootstrap", "featureflags"}
)

type cite struct {
	pkg, name, source string
}

func isReplaced(name string) bool {
	return replacedNames[name] || strings.HasPrefix(name, "TestWithTimeout")
}

func repoRoot(t *testing.T) (string, bool) {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
		if st, err := os.Stat(filepath.Join(dir, "openspec")); err == nil && st.IsDir() {
			return dir, true
		}
	}
}

func collectCites(t *testing.T, root string) []cite {
	t.Helper()
	var dirs []string
	dirs = append(dirs, filepath.Join(root, "openspec", "specs"))
	entries, err := os.ReadDir(filepath.Join(root, "openspec", "changes"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "archive" {
			dirs = append(dirs, filepath.Join(root, "openspec", "changes", e.Name()))
		}
	}
	var out []cite
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
				return err
			}
			if de.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			for _, line := range strings.Split(string(b), "\n") {
				for {
					i := strings.Index(line, "team-promotion")
					if i < 0 {
						break
					}
					seg := line[i+len("team-promotion"):]
					if j := strings.Index(seg, "team-"); j >= 0 {
						seg = seg[:j]
					}
					if j := strings.Index(seg, ";"); j >= 0 {
						seg = seg[:j]
					}
					for _, m := range pairRe.FindAllStringSubmatch(seg, -1) {
						out = append(out, cite{pkg: m[1], name: m[2], source: rel})
					}
					line = line[i+len("team-promotion"):]
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func definedTests(t *testing.T) map[string]map[string]bool {
	t.Helper()
	defs := map[string]map[string]bool{}
	base := filepath.Join("..", "..")
	err := filepath.WalkDir(base, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		rel = filepath.ToSlash(rel)
		pkg := ""
		if parts := strings.Split(rel, "/"); len(parts) > 2 && parts[0] == "internal" {
			pkg = parts[1]
		}
		for _, m := range funcRe.FindAllStringSubmatch(string(b), -1) {
			if defs[m[1]] == nil {
				defs[m[1]] = map[string]bool{}
			}
			defs[m[1]][pkg] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return defs
}

// Scenario: Every cited test name exists.
func TestSpecCitedTestsExist(t *testing.T) {
	root, ok := repoRoot(t)
	if !ok {
		return
	}
	defs := definedTests(t)
	cites := collectCites(t, root)
	if len(cites) == 0 {
		t.Fatal("no team-promotion citations found under openspec: the scan is broken")
	}
	var missing []string
	for _, c := range cites {
		if isReplaced(c.name) {
			continue
		}
		pkgs, found := defs[c.name]
		if !found {
			missing = append(missing, c.source+": internal/"+c.pkg+" "+c.name+" (not found)")
			continue
		}
		if strings.Contains(c.source, "openspec/changes/"+thisChange+"/") && !pkgs[c.pkg] {
			missing = append(missing, c.source+": "+c.name+" is not in internal/"+c.pkg)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("cited tests missing:\n%s", strings.Join(missing, "\n"))
	}
}

// Scenario: Removed packages are gone.
func TestRemovedPackagesAreGone(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "internal"))
	if err != nil {
		t.Fatal(err)
	}
	present := map[string]bool{}
	for _, e := range entries {
		present[e.Name()] = true
	}
	for _, p := range removedPackages {
		if present[p] {
			t.Errorf("internal/%s must not exist", p)
		}
	}
	for _, want := range []string{"app", "config", "store", "events", "upstream", "listing", "voucher", "flashsale", "subscription", "sponsored", "rpc", "docs"} {
		if !present[want] {
			t.Errorf("internal/%s must exist", want)
		}
	}
}

// Scenario: Every cited test name exists (archive-time citation map).
func TestCitationMapCoversEveryCitation_EveryCitedTestNameExists(t *testing.T) {
	root, ok := repoRoot(t)
	if !ok {
		return
	}
	b, err := os.ReadFile(filepath.Join(root, "openspec", "changes", thisChange, "test-package-map.txt"))
	if err != nil {
		t.Fatalf("test-package-map.txt: %v", err)
	}
	defs := definedTests(t)
	mapped := map[string]bool{}
	for i, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			t.Errorf("line %d: want `<old package> <TestName> <new package>`, got %q", i+1, line)
			continue
		}
		oldPkg := strings.TrimPrefix(f[0], "internal/")
		newPkg := strings.TrimPrefix(f[2], "internal/")
		if !defs[f[1]][newPkg] {
			t.Errorf("line %d: %s is not defined in internal/%s", i+1, f[1], newPkg)
		}
		mapped[oldPkg+" "+f[1]] = true
	}
	var missing []string
	for _, c := range collectCites(t, root) {
		if strings.Contains(c.source, "openspec/changes/"+thisChange+"/") || isReplaced(c.name) {
			continue
		}
		if !mapped[c.pkg+" "+c.name] {
			missing = append(missing, c.pkg+" "+c.name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("citations without a map line:\n%s", strings.Join(missing, "\n"))
	}
}
