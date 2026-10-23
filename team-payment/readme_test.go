package readmedoc

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func section(t *testing.T, md, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^## ` + regexp.QuoteMeta(name) + `\s*$`)
	loc := re.FindStringIndex(md)
	if loc == nil {
		t.Fatalf("README has no %q section", name)
	}
	rest := md[loc[1]:]
	if n := regexp.MustCompile(`(?m)^## `).FindStringIndex(rest); n != nil {
		rest = rest[:n[0]]
	}
	return rest
}

var removedKeys = []string{
	"DATABASE_ENABLED", "KAFKA_ENABLED", "MOCK_PAYMENTS", "in-memory", "PAYMENT_SETTLEMENT_CONSUMER_ENABLED",
	"OUTBOX_ENABLED", "SHUTDOWN_GRACE_SECONDS",
}

var oldPackages = []string{
	"internal/handler", "internal/interceptor", "internal/grpcserver", "internal/repository",
	"internal/service", "internal/consumer", "internal/bootstrap",
}

func TestReadmeNoRemovedConfigOutsideRemovedNotes_SmokeWholeService(t *testing.T) {
	md := read(t, "README.md")
	for i, line := range strings.Split(md, "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "removed") || strings.Contains(low, "no longer") {
			continue
		}
		for _, bad := range removedKeys {
			if strings.Contains(line, bad) {
				t.Errorf("README.md:%d mentions %q outside a removed note", i+1, bad)
			}
		}
	}
}

func TestReadmeLayoutUsesAdr0016Packages_SmokeWholeService(t *testing.T) {
	md := read(t, "README.md")
	for _, p := range []string{
		"internal/app", "internal/rpc", "internal/store", "internal/payment",
		"internal/settlement", "internal/upstream", "internal/events", "internal/config",
	} {
		if !strings.Contains(md, p) {
			t.Errorf("README missing package %s", p)
		}
	}
	for _, p := range oldPackages {
		if strings.Contains(md, p) {
			t.Errorf("README still mentions old package %s", p)
		}
	}
}

func TestReadmeConfigListsEveryEnvExampleKey_SmokeWholeService(t *testing.T) {
	md := read(t, "README.md")
	cfg := section(t, md, "Configuration")
	n := 0
	for _, line := range strings.Split(read(t, ".env.example"), "\n") {
		k, _, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		n++
		if !strings.Contains(cfg, "`"+k+"`") {
			t.Errorf("Configuration section does not list %s", k)
		}
	}
	if n == 0 {
		t.Error(".env.example has no variables")
	}
	for _, k := range []string{"MOCK_DATA", "LOG_KEY_FP_SECRET", "UPSTREAM_AUDIT_ADDR", "UPSTREAM_ORDER_ADDR", "DATABASE_URL"} {
		if !strings.Contains(cfg, "`"+k+"`") {
			t.Errorf("Configuration section does not list %s", k)
		}
	}
	if !strings.Contains(cfg, "payment-provider") {
		t.Error("Configuration must name the mock adapter payment-provider")
	}
}

func TestReadmeHealthSection_SmokeWholeService(t *testing.T) {
	h := section(t, read(t, "README.md"), "Health")
	for _, w := range []string{"NOT_SERVING", "postgres", "liveness", "Kafka"} {
		if !strings.Contains(h, w) {
			t.Errorf("Health section missing %q", w)
		}
	}
}

func TestReadmeMigrateAndOutboxUpgradeOrder_SmokeWholeService(t *testing.T) {
	md := read(t, "README.md")
	if !strings.Contains(md, "server migrate up") {
		t.Fatal("README must document `server migrate up`")
	}
	low := strings.ToLower(md)
	for _, w := range []string{"scale", "old pods", "never migrates", "payment_outbox_events"} {
		if !strings.Contains(low, strings.ToLower(w)) {
			t.Errorf("README outbox upgrade/migrate notes missing %q", w)
		}
	}
	if !regexp.MustCompile(`(?is)scale.{0,300}to 0.{0,400}server migrate up.{0,300}new image`).MatchString(md) {
		t.Error("README must state the order: scale old pods to 0, run `server migrate up`, start the new image")
	}
}

func TestReadmeMockPaymentsDocumentedViaMockData_SmokeWholeService(t *testing.T) {
	md := read(t, "README.md")
	if !strings.Contains(md, "MOCK_DATA=payment-provider") {
		t.Error("README must document MOCK_DATA=payment-provider")
	}
}

func TestReadmeRunLocallyUsesMakeCheck_SmokeWholeService(t *testing.T) {
	r := section(t, read(t, "README.md"), "Run locally")
	if !strings.Contains(r, "make check") {
		t.Error("Run locally missing make check")
	}
}

func TestCoverageSummaryHasNoRemovedPackages_SmokeWholeService(t *testing.T) {
	cs := read(t, "coverage.summary")
	for _, p := range oldPackages {
		if strings.Contains(cs, p+"/") {
			t.Errorf("coverage.summary still lists removed package %s", p)
		}
	}
	for _, p := range []string{"internal/app/", "internal/rpc/", "internal/store/", "internal/payment/", "internal/settlement/"} {
		if !strings.Contains(cs, p) {
			t.Errorf("coverage.summary has no entry for %s", p)
		}
	}
}
