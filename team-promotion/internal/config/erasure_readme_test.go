package config_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

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
	// PRO-1, PRO-3, PRO-5, PRO-9: Events section documents the UserErased consumer
	ev := readmeSection(t, "Events")
	for _, want := range []string{
		"identity.events", "platform.identity.v1.UserErased", "team-promotion.erasure",
		"identity.events.promotion.dlq", "deactivated_at", "new_user_voucher_state", "owner_user_id",
	} {
		if !strings.Contains(ev, want) {
			t.Errorf("README Events section does not mention %q", want)
		}
	}
	if strings.Contains(ev, "nothing is consumed") {
		t.Error("README Events section still says nothing is consumed")
	}
}

func TestErasureReadmeContractStatesNoApiPath(t *testing.T) {
	// PRO-8, PRO-9: erasure is event-driven only; the voucher API has no erasure RPC
	c := readmeSection(t, "Contract")
	for _, want := range []string{"UserErased", "identity.events"} {
		if !strings.Contains(c, want) {
			t.Errorf("README Contract section does not mention %q", want)
		}
	}
}
