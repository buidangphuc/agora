package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-promotion/internal/config"
)

var newUserDeployKeys = []string{
	"UPSTREAM_ORDER_ADDR", "UPSTREAM_ORDER_TIMEOUT",
	"NEW_USER_WELCOME_AMOUNT", "NEW_USER_INVITED_AMOUNT", "NEW_USER_MIN_SPEND", "NEW_USER_VOUCHER_TTL",
	"NEW_USER_VERDICT_TIMEOUT", "NEW_USER_VERDICT_SWEEP_INTERVAL", "VOUCHER_RESERVATION_TTL",
}

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", ".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func composeServiceBlock(t *testing.T, compose, name string) string {
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

func composeEnv(block, key string) (string, bool) {
	m := regexp.MustCompile(`(?m)^\s*-\s*` + key + `=(.*)$`).FindStringSubmatch(block)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

func yamlEnv(content, key string) (string, bool) {
	m := regexp.MustCompile(`(?m)^\s*` + key + `:\s*["']?([^"'#\n]*?)["']?\s*(#.*)?$`).FindStringSubmatch(content)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func TestEnvExampleCoversNewUserSettings(t *testing.T) {
	path := filepath.Join("..", "..", ".env.example")
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatalf("CheckEnvExample: %v", err)
	}
	body := repoFile(t, "team-promotion", ".env.example")
	for _, k := range newUserDeployKeys {
		if !regexp.MustCompile(`(?m)^` + k + `=`).MatchString(body) {
			t.Errorf(".env.example lacks %s", k)
		}
	}
}

func TestComposeWiresNewUserVouchers(t *testing.T) {
	compose := repoFile(t, "docker-compose.services.yaml")
	block := composeServiceBlock(t, compose, "team-promotion")

	if v, ok := composeEnv(block, "UPSTREAM_ORDER_ADDR"); !ok || v != "team-order-svc:50055" {
		t.Errorf("compose team-promotion UPSTREAM_ORDER_ADDR = %q (set=%v), want team-order-svc:50055", v, ok)
	}
	for _, k := range newUserDeployKeys {
		if _, ok := composeEnv(block, k); !ok {
			t.Errorf("compose team-promotion does not set %s", k)
		}
	}
	for k, want := range map[string]string{
		"NEW_USER_WELCOME_AMOUNT": "20000", "NEW_USER_INVITED_AMOUNT": "30000", "NEW_USER_MIN_SPEND": "100000",
	} {
		if v, ok := composeEnv(block, k); ok && v != want {
			t.Errorf("compose team-promotion %s = %q, want %s", k, v, want)
		}
	}
	for _, dep := range []string{"team-order", "redpanda-init"} {
		if !regexp.MustCompile(`(?m)^\s+` + dep + `:\s*$`).MatchString(block) {
			t.Errorf("compose team-promotion does not list %s in depends_on", dep)
		}
	}
	if strings.Contains(block, "DATABASE_ENABLED") || strings.Contains(block, "KAFKA_ENABLED") {
		t.Error("compose team-promotion sets a retired enable flag")
	}

	var topicLine string
	for _, l := range strings.Split(compose, "\n") {
		if strings.Contains(l, "rpk topic create") {
			topicLine = l
		}
	}
	if topicLine == "" {
		t.Fatal("compose has no rpk topic create line")
	}
	fields := strings.Fields(topicLine)
	for _, topic := range []string{"identity.events", "referral.events", "identity.events.promotion.dlq", "referral.events.promotion.dlq"} {
		found := false
		for _, f := range fields {
			if f == topic {
				found = true
			}
		}
		if !found {
			t.Errorf("redpanda-init does not create topic %s", topic)
		}
	}
}

func TestGitopsWiresNewUserVouchers(t *testing.T) {
	baseline := repoFile(t, "platform-gitops", "envs", "services", "team-promotion.yaml")
	argocd := repoFile(t, "platform-gitops", "argocd", "apps", "team-promotion.yaml")
	for name, content := range map[string]string{"baseline": baseline, "argocd": argocd} {
		for _, k := range newUserDeployKeys {
			if _, ok := yamlEnv(content, k); !ok {
				t.Errorf("%s team-promotion does not set %s", name, k)
			}
		}
		if v, ok := yamlEnv(content, "UPSTREAM_ORDER_ADDR"); !ok || !strings.HasPrefix(v, "team-order:") {
			t.Errorf("%s UPSTREAM_ORDER_ADDR = %q, want team-order:<port>", name, v)
		}
	}
}

func TestOrderNetworkPolicyAllowsPromotion(t *testing.T) {
	order := repoFile(t, "platform-gitops", "envs", "services", "team-order.yaml")
	m := regexp.MustCompile(`(?m)^\s*allowFrom:\s*\[([^\]]*)\]`).FindStringSubmatch(order)
	if m == nil {
		t.Fatal("team-order.yaml has no networkPolicy.allowFrom list")
	}
	var got []string
	for _, p := range strings.Split(m[1], ",") {
		got = append(got, strings.TrimSpace(p))
	}
	for _, want := range []string{"team-payment", "team-engagement", "team-promotion"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("team-order allowFrom = %v, missing %s", got, want)
		}
	}
}
