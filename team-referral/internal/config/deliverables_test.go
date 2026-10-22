package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-referral/internal/config"
)

// Deliverables of tasks 1.5 (Makefile, .env.example), 1.6 (compose), 1.7 (gitops).
// Red until those tasks land; not part of task 1.1 green.

func TestMakefileUsesSharedFiles(t *testing.T) {
	// scenario: The Makefile uses the shared files
	root := serviceRoot(t)
	mk := readFile(t, filepath.Join(root, "Makefile"))
	for _, want := range []string{
		"include ../platform-core/make/go-service.mk",
		"LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml",
	} {
		if !strings.Contains(mk, want) {
			t.Errorf("Makefile lacks %q", want)
		}
	}
	for _, p := range []string{".golangci.yml", "make"} {
		if _, err := os.Lstat(filepath.Join(root, p)); err == nil {
			t.Errorf("%s must not exist", p)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestEnvExampleInSync(t *testing.T) {
	// scenario: The env example matches the settings
	path := filepath.Join(serviceRoot(t), ".env.example")
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatalf("CheckEnvExample: %v", err)
	}
	if strings.Contains(readFile(t, path), "DATABASE_ENABLED") {
		t.Error(".env.example still contains DATABASE_ENABLED")
	}
}

func composeBlock(t *testing.T, compose, name string) string {
	t.Helper()
	lines := strings.Split(compose, "\n")
	start := -1
	for i, l := range lines {
		if l == "  "+name+":" {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("docker-compose.services.yaml has no service %s", name)
	}
	end := len(lines)
	next := regexp.MustCompile(`^  [A-Za-z0-9_-]+:\s*$`)
	for i := start + 1; i < len(lines); i++ {
		if next.MatchString(lines[i]) {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func TestDeployFilesCarryNoRemovedFlag(t *testing.T) {
	// scenario: Compose and gitops carry no removed flag
	repo := filepath.Join(serviceRoot(t), "..")

	block := composeBlock(t, readFile(t, filepath.Join(repo, "docker-compose.services.yaml")), "team-referral")
	if strings.Contains(block, "DATABASE_ENABLED") {
		t.Error("compose team-referral still sets DATABASE_ENABLED")
	}
	for _, want := range []string{"ENV=", "GRPC_PORT="} {
		if !strings.Contains(block, want) {
			t.Errorf("compose team-referral lacks %s", want)
		}
	}

	files := map[string]string{
		"baseline": filepath.Join(repo, "platform-gitops", "envs", "services", "team-referral.yaml"),
		"argocd":   filepath.Join(repo, "platform-gitops", "argocd", "apps", "team-referral.yaml"),
		"staging":  filepath.Join(repo, "platform-gitops", "envs", "staging", "services", "team-referral.yaml"),
		"prod":     filepath.Join(repo, "platform-gitops", "envs", "prod", "services", "team-referral.yaml"),
	}
	envKey := regexp.MustCompile(`(?m)^\s*ENV:\s*["']?[A-Za-z]+["']?\s*(#.*)?$`)
	portKey := regexp.MustCompile(`(?m)^\s*GRPC_PORT:\s*["']?50062["']?\s*(#.*)?$`)
	liveness := regexp.MustCompile(`(?m)^\s*livenessService:\s*["']?liveness["']?\s*(#.*)?$`)
	for name, path := range files {
		content := readFile(t, path)
		if strings.Contains(content, "DATABASE_ENABLED") {
			t.Errorf("%s: contains DATABASE_ENABLED", name)
		}
		switch name {
		case "baseline", "argocd":
			if !envKey.MatchString(content) || !portKey.MatchString(content) {
				t.Errorf("%s: must set ENV and GRPC_PORT: 50062", name)
			}
			if !liveness.MatchString(content) {
				t.Errorf("%s: must set probes.livenessService to liveness", name)
			}
		case "staging":
			if !regexp.MustCompile(`(?m)^\s*ENV:\s*["']?staging["']?`).MatchString(content) {
				t.Errorf("%s: must set ENV to staging", name)
			}
		case "prod":
			if !regexp.MustCompile(`(?m)^\s*ENV:\s*["']?production["']?`).MatchString(content) {
				t.Errorf("%s: must set ENV to production", name)
			}
		}
	}
}
