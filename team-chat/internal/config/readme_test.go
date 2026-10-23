package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func readTeamChatFile(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func TestReadmeHasOutboxDeploySection_Task132(t *testing.T) {
	readme := readTeamChatFile(t, "README.md")
	if n := strings.Count(readme, "Deploying the go-sdk v1 outbox"); n != 1 {
		t.Fatalf("README must contain the heading 'Deploying the go-sdk v1 outbox' exactly once, got %d", n)
	}
	for _, want := range []string{"migrate up"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README must mention %q", want)
		}
	}
}

func TestReadmeAndEnvExampleHaveNoLegacyEnvNames_Task132(t *testing.T) {
	re := regexp.MustCompile(`SERVER_PORT|POSTGRES_HOST|KAFKA_ENABLED|OUTBOX_ENABLED`)
	for _, f := range []string{"README.md", ".env.example"} {
		if m := re.FindAllString(readTeamChatFile(t, f), -1); len(m) > 0 {
			t.Errorf("%s still holds legacy env names: %v", f, m)
		}
	}
}
