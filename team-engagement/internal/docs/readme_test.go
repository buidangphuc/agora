package docs

import (
	"os"
	"strings"
	"testing"
)

func readme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Scenario: Review hidden event: effective hide enqueues one fact.
func TestReadme_ListsReviewHiddenAmongPublishedFacts_EffectiveHideEnqueuesOneFact(t *testing.T) {
	for _, line := range strings.Split(readme(t), "\n") {
		if strings.Contains(line, "`ReviewCreated`") && strings.Contains(line, "`ReviewHidden`") {
			return
		}
	}
	t.Fatal("README facts list must include `ReviewHidden` next to `ReviewCreated`")
}

// Scenario: Review hidden event: effective hide enqueues one fact (deploy order note).
func TestReadme_NotesAnalyticsFirstDeployOrder_EffectiveHideEnqueuesOneFact(t *testing.T) {
	for _, line := range strings.Split(readme(t), "\n") {
		l := strings.ToLower(line)
		if strings.Contains(line, "ReviewHidden") && strings.Contains(l, "analytics") && strings.Contains(l, "before") {
			return
		}
	}
	t.Fatal("README must note that team-analytics deploys before team-engagement for ReviewHidden")
}

func envRow(t *testing.T, name string) string {
	t.Helper()
	for _, line := range strings.Split(readme(t), "\n") {
		if strings.HasPrefix(line, "|") && strings.Contains(line, "`"+name+"`") {
			return line
		}
	}
	t.Fatalf("README env table has no row for %s", name)
	return ""
}

// Scenario: Engagement SDK: Old packages are gone (README layout section).
func TestReadme_DescribesADR0016Layout_OldPackagesAreGone(t *testing.T) {
	r := readme(t)
	for _, p := range []string{"internal/app", "internal/config", "internal/store", "internal/engagement", "internal/upstream", "internal/rpc", "internal/events"} {
		if !strings.Contains(r, p) {
			t.Errorf("README must describe %s", p)
		}
	}
	for _, p := range []string{"internal/bootstrap", "internal/grpcserver", "internal/observability", "internal/interceptor", "internal/handler", "internal/service", "internal/repository", "internal/consumer", "interceptor.RequireScopes"} {
		if strings.Contains(r, p) {
			t.Errorf("README must not mention removed package %s", p)
		}
	}
}

// Scenario: Engagement SDK: No fallback code in the binary (README drops flags).
func TestReadme_DropsEnableFlagsAndFallbacks_NoFallbackCodeInTheBinary(t *testing.T) {
	r := readme(t)
	for _, s := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "UPSTREAM_CALL_TIMEOUT_SECONDS", "host.docker.internal", "Empty disables verified-purchase", "silently disables purchase verification"} {
		if strings.Contains(r, s) {
			t.Errorf("README must not mention %q", s)
		}
	}
	for _, name := range []string{"OTEL_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		envRow(t, name)
	}
}

// Scenario: Engagement SDK: No fallback code in the binary (required addresses).
func TestReadme_MarksPortAndUpstreamsRequired_NoFallbackCodeInTheBinary(t *testing.T) {
	for _, name := range []string{"GRPC_PORT", "UPSTREAM_AUDIT_ADDR", "UPSTREAM_ORDER_ADDR", "DATABASE_URL"} {
		if !strings.Contains(strings.ToLower(envRow(t, name)), "required") {
			t.Errorf("README row for %s must say it is required", name)
		}
	}
}

// Scenario: Engagement SDK: Old packages are gone (README operations section).
func TestReadme_DocumentsMigrateUpHealthOutboxAndCheck_OldPackagesAreGone(t *testing.T) {
	r := readme(t)
	for _, s := range []string{"migrate up", "`liveness`", "`postgres`", "outbox", "make check", "LOG_KEY_FP_SECRET"} {
		if !strings.Contains(r, s) {
			t.Errorf("README must mention %q", s)
		}
	}
	if strings.Contains(r, "never by the server") && !strings.Contains(r, "migrate up") {
		t.Error("README must describe the migrate up subcommand")
	}
}
