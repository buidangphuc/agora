package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readServiceFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(serviceRoot(t), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

func TestMakefileUsesSharedBuildFiles(t *testing.T) {
	mk := readServiceFile(t, "Makefile")

	for _, re := range []string{
		`(?m)^SERVICE\s*:=\s*team-domain\s*$`,
		`(?m)^GRPC_PORT\s*:=\s*50051\s*$`,
		`(?m)^BINARIES\s*:=\s*server\s*$`,
		`(?m)^LINT_CONFIG\s*:=\s*\.\./platform-core/packages/go-sdk/\.golangci\.yml\s*$`,
		`(?m)^include\s+\.\./platform-core/make/go-service\.mk\s*$`,
	} {
		if !regexp.MustCompile(re).MatchString(mk) {
			t.Errorf("Makefile does not match %s", re)
		}
	}

	for _, target := range []string{"check", "check-env", "test", "migrate", "proto", "run"} {
		if regexp.MustCompile(`(?m)^` + target + `\s*:`).MatchString(mk) {
			t.Errorf("Makefile still defines local target %q provided by the include", target)
		}
	}
	for _, target := range []string{"dev", "stop"} {
		if !regexp.MustCompile(`(?m)^` + target + `\s*:`).MatchString(mk) {
			t.Errorf("Makefile lost target %q", target)
		}
	}

	root := serviceRoot(t)
	for _, gone := range []string{".golangci.yml", "make"} {
		if _, err := os.Stat(filepath.Join(root, gone)); !os.IsNotExist(err) {
			t.Errorf("team-domain/%s must not exist (stat err: %v)", gone, err)
		}
	}
}

func TestDockerfileFollowsServiceTemplate(t *testing.T) {
	df := readServiceFile(t, "Dockerfile")

	if !regexp.MustCompile(`(?m)^ARG BIN=server\s*$`).MatchString(df) {
		t.Errorf("Dockerfile does not declare ARG BIN=server")
	}
	if !regexp.MustCompile(`(?m)^ENTRYPOINT\s*\[\s*"/app"\s*\]\s*$`).MatchString(df) {
		t.Errorf("Dockerfile entrypoint is not /app")
	}
	if !strings.Contains(df, "/cmd/${BIN}") && !strings.Contains(df, "./cmd/$BIN") && !strings.Contains(df, "./cmd/${BIN}") {
		t.Errorf("Dockerfile does not build ./cmd/${BIN}")
	}
	if strings.Contains(df, "third_party") {
		t.Errorf("Dockerfile references third_party")
	}
	if strings.Contains(df, `"/server"`) {
		t.Errorf("Dockerfile still uses the /server entrypoint")
	}
}

func TestReadmeDropsEnableFlags(t *testing.T) {
	readme := readServiceFile(t, "README.md")
	for _, flag := range []string{"KAFKA_ENABLED", "OUTBOX_ENABLED", "IDENTITY_CONSUMER_ENABLED", "DATABASE_ENABLED"} {
		if strings.Contains(readme, flag) {
			t.Errorf("README still mentions %s", flag)
		}
	}
	for _, want := range []string{"migrate up", "audit trail unavailable", "missing scope"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README does not mention %q", want)
		}
	}
}
