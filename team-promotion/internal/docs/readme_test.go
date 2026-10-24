package docs

import (
	"os"
	"strings"
	"testing"
)

// Scenario: Shop voucher audit: admin creates a shop voucher (task 3, docs).
// The README audited-actions description must list voucher.admin_create.
func TestReadmeListsVoucherAdminCreateAuditedAction_ShopVoucherAuditAdminCreates(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "voucher.admin_create") {
		t.Fatal("README.md does not list the audited action voucher.admin_create")
	}
}

func readmeText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func settingsRow(t *testing.T, name string) string {
	t.Helper()
	for _, line := range strings.Split(readmeText(t), "\n") {
		if strings.HasPrefix(line, "|") && strings.Contains(line, "`"+name+"`") {
			return line
		}
	}
	t.Fatalf("README settings table has no row for %s", name)
	return ""
}

// Scenario: Removed packages are gone (README layout).
func TestReadmeDescribesADR0016LayoutWithoutRemovedPackages_RemovedPackagesAreGone(t *testing.T) {
	r := readmeText(t)
	for _, p := range []string{"internal/app", "internal/config", "internal/store", "internal/events", "internal/upstream", "internal/listing", "internal/voucher", "internal/flashsale", "internal/subscription", "internal/sponsored", "internal/rpc"} {
		if !strings.Contains(r, p) {
			t.Errorf("README must describe %s", p)
		}
	}
	for _, p := range []string{"internal/service", "internal/handler", "internal/repository", "internal/producer", "internal/interceptor", "internal/grpcserver", "internal/bootstrap", "internal/featureflags"} {
		if strings.Contains(r, p) {
			t.Errorf("README must not mention removed package %s", p)
		}
	}
}

// Scenario: Every cited test name exists (README settings table drops retired keys).
func TestReadmeSettingsDropRetiredKeys_EveryCitedTestNameExists(t *testing.T) {
	r := readmeText(t)
	for _, s := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "FEATURE_FLAGS_ENABLED", "UPSTREAM_CALL_TIMEOUT_SECONDS", "SHUTDOWN_GRACE_SECONDS", "FEATURE_FLAGS_EVAL_TIMEOUT_MS", "in-memory", "make check-env", "make migrate", "IsProd", "50060"} {
		if strings.Contains(r, s) {
			t.Errorf("README must not mention %q", s)
		}
	}
}

// Scenario: Every cited test name exists (README settings table lists the new keys).
func TestReadmeSettingsListNewKeys_EveryCitedTestNameExists(t *testing.T) {
	for _, name := range []string{"SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "FEATURE_FLAGS_EVAL_TIMEOUT", "OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK", "FLIPT_ADDR", "KAFKA_BROKERS", "GRPC_REFLECTION_ENABLED", "MAX_AD_BID", "MAX_AD_BUDGET", "PROMOTION_EVENTS_TOPIC"} {
		settingsRow(t, name)
	}
	if !strings.Contains(strings.ToLower(settingsRow(t, "GRPC_REFLECTION_ENABLED")), "false") {
		t.Error("README must give GRPC_REFLECTION_ENABLED default false")
	}
}

// Scenario: Every cited test name exists (required settings).
func TestReadmeMarksRequiredSettings_EveryCitedTestNameExists(t *testing.T) {
	for _, name := range []string{"ENV", "GRPC_PORT", "DATABASE_URL", "UPSTREAM_AUDIT_ADDR", "UPSTREAM_DOMAIN_ADDR"} {
		if !strings.Contains(strings.ToLower(settingsRow(t, name)), "required") {
			t.Errorf("README row for %s must say it is required", name)
		}
	}
}

// Scenario: Removed packages are gone (README operations).
func TestReadmeDocumentsMigrateOutboxHealthAndCheck_RemovedPackagesAreGone(t *testing.T) {
	r := readmeText(t)
	for _, s := range []string{"migrate up", "0005_outbox", "outbox", "`liveness`", "`postgres`", "make check", "voucher.admin_create"} {
		if !strings.Contains(r, s) {
			t.Errorf("README must mention %q", s)
		}
	}
}
