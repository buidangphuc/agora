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
	body := readFile(t, path)
	if strings.Contains(body, "DATABASE_ENABLED") {
		t.Error(".env.example still contains DATABASE_ENABLED")
	}
	for _, k := range []string{"KAFKA_BROKERS", "OUTBOX_POLL_INTERVAL", "REFERRAL_REWARD_XU", "REFERRAL_DAILY_CAP", "REFERRAL_LIFETIME_CAP", "REFERRAL_CAP_TIMEZONE", "REFERRAL_EVENTS_TOPIC", "REFERRAL_ORDER_DLQ_TOPIC"} {
		if !regexp.MustCompile(`(?m)^` + k + `=`).MatchString(body) {
			t.Errorf(".env.example lacks %s", k)
		}
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
	anyEnabled := regexp.MustCompile(`[A-Z][A-Z0-9_]*_ENABLED`)
	allowedEnabled := map[string]bool{"OTEL_ENABLED": true, "GRPC_REFLECTION_ENABLED": true}
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
	for _, f := range anyEnabled.FindAllString(block, -1) {
		if !allowedEnabled[f] {
			t.Errorf("compose team-referral sets removed flag %s", f)
		}
	}
	for name, path := range files {
		content := readFile(t, path)
		for _, f := range anyEnabled.FindAllString(content, -1) {
			if !allowedEnabled[f] {
				t.Errorf("%s: contains removed flag %s", name, f)
			}
		}
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

var kafkaDeployKeys = []string{
	"KAFKA_BROKERS", "IDENTITY_EVENTS_TOPIC", "ORDER_EVENTS_TOPIC", "REFERRAL_EVENTS_TOPIC",
	"REFERRAL_IDENTITY_CONSUMER_GROUP", "REFERRAL_ORDER_CONSUMER_GROUP",
	"REFERRAL_IDENTITY_DLQ_TOPIC", "REFERRAL_ORDER_DLQ_TOPIC",
	"OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK",
}

func composeValue(block, key string) (string, bool) {
	m := regexp.MustCompile(`(?m)^\s*-\s*` + key + `=(.*)$`).FindStringSubmatch(block)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

func gitopsValue(content, key string) (string, bool) {
	m := regexp.MustCompile(`(?m)^\s*` + key + `:\s*["']?([^"'#\n]*?)["']?\s*(#.*)?$`).FindStringSubmatch(content)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func TestDeployFilesCarryKafkaSettings(t *testing.T) {
	// scenario: The deploy files carry the Kafka and referral settings
	repo := filepath.Join(serviceRoot(t), "..")
	composeFile := readFile(t, filepath.Join(repo, "docker-compose.services.yaml"))
	block := composeBlock(t, composeFile, "team-referral")
	baseline := readFile(t, filepath.Join(repo, "platform-gitops", "envs", "services", "team-referral.yaml"))

	for _, k := range kafkaDeployKeys {
		if _, ok := composeValue(block, k); !ok {
			t.Errorf("compose team-referral does not set %s", k)
		}
		if _, ok := gitopsValue(baseline, k); !ok {
			t.Errorf("gitops baseline does not set %s", k)
		}
	}
	if !regexp.MustCompile(`(?m)^\s+redpanda-init:\s*$`).MatchString(block) {
		t.Error("compose team-referral does not list redpanda-init in depends_on")
	}

	var topicLine string
	for _, l := range strings.Split(composeFile, "\n") {
		if strings.Contains(l, "rpk topic create") {
			topicLine = l
		}
	}
	if topicLine == "" {
		t.Fatal("compose has no rpk topic create line")
	}
	fields := strings.Fields(topicLine)
	has := func(topic string) bool {
		for _, f := range fields {
			if f == topic {
				return true
			}
		}
		return false
	}

	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50062")
	t.Setenv("DATABASE_URL", testDSN)
	cleanEnvKeepRequired(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load defaults: %v", err)
	}
	for _, topic := range []string{s.Referral.EventsTopic, s.Referral.IdentityDLQTopic, s.Referral.OrderDLQTopic} {
		if !has(topic) {
			t.Errorf("compose rpk topic create line does not create %s", topic)
		}
	}
	for _, topic := range []string{"referral.events", "referral.events.dlq", "referral.identity.dlq", "referral.order.dlq"} {
		if !has(topic) {
			t.Errorf("compose rpk topic create line does not create %s", topic)
		}
	}
	for key, want := range map[string]string{
		"REFERRAL_EVENTS_TOPIC":       s.Referral.EventsTopic,
		"REFERRAL_IDENTITY_DLQ_TOPIC": s.Referral.IdentityDLQTopic,
		"REFERRAL_ORDER_DLQ_TOPIC":    s.Referral.OrderDLQTopic,
		"IDENTITY_EVENTS_TOPIC":       s.Referral.IdentityTopic,
		"ORDER_EVENTS_TOPIC":          s.Referral.OrderTopic,
	} {
		if got, ok := composeValue(block, key); ok && got != want {
			t.Errorf("compose %s = %q, config default is %q", key, got, want)
		}
		if got, ok := gitopsValue(baseline, key); ok && got != want {
			t.Errorf("gitops %s = %q, config default is %q", key, got, want)
		}
	}
	for _, f := range regexp.MustCompile(`[A-Z][A-Z0-9_]*_ENABLED`).FindAllString(block+baseline, -1) {
		if f != "OTEL_ENABLED" && f != "GRPC_REFLECTION_ENABLED" {
			t.Errorf("deploy files contain removed flag %s", f)
		}
	}
}

func cleanEnvKeepRequired(t *testing.T) {
	t.Helper()
	for _, k := range managedKeys {
		switch k {
		case "ENV", "GRPC_PORT", "DATABASE_URL":
			continue
		}
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}
