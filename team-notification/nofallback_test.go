package notification_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTask19NoFallbackMarkersInProductionFiles(t *testing.T) {
	re := regexp.MustCompile(`DATABASE_ENABLED|KAFKA_ENABLED|InMemory|Noop|mock mode|lookup disabled`)
	var hits []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if re.MatchString(line) {
				hits = append(hits, p+":"+string(rune('0'+i%10))+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("task 1.9 no-fallback grep must return nothing:\n%s", strings.Join(hits, "\n"))
	}
}
