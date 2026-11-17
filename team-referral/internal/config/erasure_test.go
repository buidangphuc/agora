package config_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/buidangphuc/team-referral/internal/config"
)

func TestErasureConsumerDefaults(t *testing.T) {
	// REF-4, REF-6: own consumer group and DLQ for the UserErased consumer
	baseEnv(t, "local")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := s.Referral
	if r.ErasureGroup != "team-referral-erasure" {
		t.Errorf("ErasureGroup = %q, want team-referral-erasure", r.ErasureGroup)
	}
	if r.ErasureDLQTopic != "identity.events.referral.dlq" {
		t.Errorf("ErasureDLQTopic = %q, want identity.events.referral.dlq", r.ErasureDLQTopic)
	}
	if r.ErasureGroup == r.IdentityGroup || r.ErasureGroup == r.OrderGroup {
		t.Errorf("ErasureGroup %q must differ from the identity and order groups", r.ErasureGroup)
	}
	if r.ErasureDLQTopic == r.IdentityDLQTopic || r.ErasureDLQTopic == r.OrderDLQTopic {
		t.Errorf("ErasureDLQTopic %q must differ from the existing DLQs", r.ErasureDLQTopic)
	}
}

func TestErasureConsumerEnvOverrides(t *testing.T) {
	// REF-4, REF-6
	baseEnv(t, "local")
	t.Setenv("REFERRAL_ERASURE_CONSUMER_GROUP", "g-x")
	t.Setenv("REFERRAL_ERASURE_DLQ_TOPIC", "dlq-x")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Referral.ErasureGroup != "g-x" || s.Referral.ErasureDLQTopic != "dlq-x" {
		t.Errorf("erasure = %q / %q, want g-x / dlq-x", s.Referral.ErasureGroup, s.Referral.ErasureDLQTopic)
	}
}

func readmeSection(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	loc := regexp.MustCompile(`(?m)^## ` + regexp.QuoteMeta(name) + `\s*$`).FindStringIndex(md)
	if loc == nil {
		t.Fatalf("README has no %q section", name)
	}
	rest := md[loc[1]:]
	if n := regexp.MustCompile(`(?m)^## `).FindStringIndex(rest); n != nil {
		rest = rest[:n[0]]
	}
	return rest
}

func TestErasureReadmeDocumentsConsumer(t *testing.T) {
	// REF-1, REF-4, REF-6, REF-8: Events and Configuration sections
	ev := readmeSection(t, "Events")
	for _, want := range []string{
		"identity.events", "platform.identity.v1.UserErased", "team-referral-erasure", "identity.events.referral.dlq",
		"referral_codes", "referral_accounts", "referral_buyer_orders", "referral_rewards", "referrals",
	} {
		if !strings.Contains(ev, want) {
			t.Errorf("README Events section does not mention %q", want)
		}
	}
	cfg := readmeSection(t, "Configuration")
	for _, want := range []string{"REFERRAL_ERASURE_CONSUMER_GROUP", "REFERRAL_ERASURE_DLQ_TOPIC"} {
		if !strings.Contains(cfg, "`"+want+"`") {
			t.Errorf("README Configuration section does not list %s", want)
		}
	}
}

func composeService(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../../docker-compose.services.yaml")
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	loc := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(name) + `:\s*$`).FindStringIndex(md)
	if loc == nil {
		t.Fatalf("compose has no service %q", name)
	}
	rest := md[loc[1]:]
	if n := regexp.MustCompile(`(?m)^  [A-Za-z0-9_-]+:\s*$`).FindStringIndex(rest); n != nil {
		rest = rest[:n[0]]
	}
	return rest
}

func TestErasureComposeTopicsAreCreated(t *testing.T) {
	// REF-6, PRO-5: the three new DLQ topics exist in the topic init
	b, err := os.ReadFile("../../../docker-compose.services.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var create string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "rpk topic create") {
			create = line
		}
	}
	if create == "" {
		t.Fatal("compose has no rpk topic create line")
	}
	have := map[string]bool{}
	for _, f := range strings.Fields(create) {
		have[f] = true
	}
	for _, topic := range []string{"identity.events.order.dlq", "identity.events.payment.dlq", "identity.events.referral.dlq", "identity.events.promotion.dlq"} {
		if !have[topic] {
			t.Errorf("topic init does not create %s", topic)
		}
	}
}

func TestErasureComposeConsumerEnv(t *testing.T) {
	// REF-4, REF-6: each service that consumes UserErased gets its group and DLQ in compose
	want := map[string][]string{
		"team-order":    {"ORDER_ERASURE_CONSUMER_GROUP=team-order.erasure", "ORDER_ERASURE_DLQ_TOPIC=identity.events.order.dlq"},
		"team-payment":  {"PAYMENT_ERASURE_CONSUMER_GROUP=team-payment.erasure", "PAYMENT_ERASURE_DLQ_TOPIC=identity.events.payment.dlq"},
		"team-referral": {"REFERRAL_ERASURE_CONSUMER_GROUP=team-referral-erasure", "REFERRAL_ERASURE_DLQ_TOPIC=identity.events.referral.dlq"},
	}
	for svc, lines := range want {
		block := composeService(t, svc)
		for _, l := range lines {
			if !strings.Contains(block, "- "+l) {
				t.Errorf("compose service %s lacks env %s", svc, l)
			}
		}
	}
}
