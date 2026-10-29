package app_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var removedPatterns = regexp.MustCompile(`KAFKA_ENABLED|SHUTDOWN_GRACE_SECONDS|GA4_EXPORT_SOURCE|GA4_RELAY_PROBE_PORT|os\.Getenv|string\(rec\.Key\)|string\(r\.Key\)`)

func TestWholeServiceGate192SanityGrepEmptyOutsideTests(t *testing.T) {
	root := serviceRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == "generated" || rel == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if rel == "FEATURES.yaml" || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if removedPatterns.MatchString(line) {
				t.Errorf("%s:%d: %s", rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
