package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var legacyNames = []string{"DATABASE_ENABLED", "NewInMemory", "OpenOrLocal", "internal/service", "internal/repository", "TEST_DATABASE_URL"}

// Scenario: The server binary does not link the in-memory store
func TestNoLegacyNamesRemainInService(t *testing.T) {
	root := serviceRoot(t)
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == filepath.Join("internal", "store", "storetest") || rel == "generated" || d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, n := range legacyNames {
				if strings.Contains(line, n) {
					hits = append(hits, rel+":"+strconv.Itoa(i+1)+": "+n)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("legacy names remain:\n%s", strings.Join(hits, "\n"))
	}
}
