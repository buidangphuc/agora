package app

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var retiredKeys = regexp.MustCompile(`KAFKA_ENABLED|EDGE_REFLECTION_ENABLED|HTTP_PORT|SHUTDOWN_GRACE_SECONDS|NoopPublisher|NewInMemory|third_party`)

func TestNoRetiredKeysInProductionFiles(t *testing.T) {
	root := serviceRoot(t)
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			switch d.Name() {
			case "generated", ".git", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(rel, "_test.go") || rel == "README.md" || rel == "FEATURES.yaml" ||
			rel == "go.sum" || rel == "coverage.summary" {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; sc.Scan(); n++ {
			if retiredKeys.MatchString(sc.Text()) {
				hits = append(hits, rel+":"+itoa(n)+": "+strings.TrimSpace(sc.Text()))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		t.Errorf("retired key in production file: %s", h)
	}
}

func itoa(n int) string {
	return intToString(n)
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func definedTests(t *testing.T, root string) map[string]bool {
	t.Helper()
	def := map[string]bool{}
	re := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "generated" {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			def[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func TestSpecCitationsResolveToExistingGatewayTests(t *testing.T) {
	svc := serviceRoot(t)
	repo := filepath.Join(svc, "..")
	def := definedTests(t, svc)
	cite := regexp.MustCompile(`\bTest\w*[A-Za-z0-9]\b`)
	pathRe := regexp.MustCompile("`(team-gateway/[^`]+_test\\.go)`")
	var specFiles []string
	for _, dir := range []string{filepath.Join(repo, "openspec", "specs"), filepath.Join(repo, "openspec", "changes", "gateway-go-sdk-v1", "specs")} {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && d.Name() == "spec.md" {
				specFiles = append(specFiles, p)
			}
			return nil
		})
	}
	if len(specFiles) == 0 {
		t.Fatal("no spec files found")
	}
	checked := 0
	var missing []string
	for _, p := range specFiles {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.Contains(line, "VERIFIED BY") || !strings.Contains(line, "team-gateway") {
				continue
			}
			if m := pathRe.FindStringSubmatch(line); m != nil {
				if _, err := os.Stat(filepath.Join(repo, m[1])); err != nil {
					missing = append(missing, p+": file "+m[1])
				}
			}
			for _, name := range cite.FindAllString(line, -1) {
				checked++
				if !def[name] {
					missing = append(missing, p+": "+name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no cited tests found: citation parser is broken")
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("cited test does not exist: %s", m)
	}
}
