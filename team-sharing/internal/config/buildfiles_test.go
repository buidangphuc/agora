package config_test

import (
	"os"
	"strings"
	"testing"
)

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Split(string(b), "\n")
}

func TestEnvExampleDeclaresExactlyTheSDKVariables(t *testing.T) {
	want := []string{
		"ENV=local",
		"LOG_LEVEL=info",
		"LOG_JSON=true",
		"GRPC_HOST=0.0.0.0",
		"GRPC_PORT=50065",
		"GRPC_REFLECTION_ENABLED=false",
		"SHUTDOWN_GRACE=10s",
		"SHUTDOWN_DRAIN_DELAY=0s",
		"DATABASE_URL=postgres://sharing_svc:sharing_pass@localhost:5447/sharing_db?sslmode=disable",
		"DB_MAX_CONNS=10",
		"OTEL_ENABLED=false",
		"OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317",
		"OTEL_SERVICE_NAME=team-sharing",
	}
	got := map[string]bool{}
	for _, l := range readLines(t, "../../.env.example") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		got[l] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf(".env.example scenario %q: missing line %q", ".env.example is in sync with Settings", w)
		}
		delete(got, w)
	}
	for extra := range got {
		t.Errorf(".env.example scenario %q: unexpected line %q", ".env.example is in sync with Settings", extra)
	}
}

func TestDockerfileBuildsFromModuleCache(t *testing.T) {
	const sc = "The image builds from the module cache"
	lines := readLines(t, "../../Dockerfile")
	has := func(s string) bool {
		for _, l := range lines {
			if strings.TrimSpace(l) == s {
				return true
			}
		}
		return false
	}
	for _, s := range []string{
		"COPY go.mod go.sum ./",
		"RUN go mod download",
		"ENV GRPC_PORT=50065",
		"USER nonroot:nonroot",
	} {
		if !has(s) {
			t.Errorf("%s: Dockerfile lacks line %q", sc, s)
		}
	}
	all := strings.Join(lines, "\n")
	if strings.Contains(all, "third_party") {
		t.Errorf("%s: Dockerfile references third_party", sc)
	}
	if strings.Contains(all, "go.sum*") {
		t.Errorf("%s: go.sum must be copied unconditionally", sc)
	}
	if !strings.Contains(all, "./cmd/server") {
		t.Errorf("%s: Dockerfile must build ./cmd/server", sc)
	}
}
