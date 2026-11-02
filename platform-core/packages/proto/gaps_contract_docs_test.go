package proto_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var allReasons = []string{"AUDIT_UNAVAILABLE", "PROMOTION_UNAVAILABLE", "LISTING_UNAVAILABLE"}

type docRule struct {
	name     string
	kind     string
	contains []string
	absent   []string
}

func readSrc(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func leadingComment(src, kind, name string) (string, bool) {
	lines := strings.Split(src, "\n")
	prefix := kind + " " + name
	for i, l := range lines {
		tl := strings.TrimSpace(l)
		if !strings.HasPrefix(tl, prefix+"(") && !strings.HasPrefix(tl, prefix+" ") && !strings.HasPrefix(tl, prefix+"{") {
			continue
		}
		var block []string
		for j := i - 1; j >= 0; j-- {
			c := strings.TrimSpace(lines[j])
			if !strings.HasPrefix(c, "//") {
				break
			}
			block = append([]string{strings.TrimPrefix(c, "//")}, block...)
		}
		return strings.Join(block, "\n"), true
	}
	return "", false
}

func checkRules(src string, rules []docRule) []string {
	var problems []string
	for _, r := range rules {
		c, ok := leadingComment(src, r.kind, r.name)
		if !ok {
			problems = append(problems, r.name+": "+r.kind+" not found")
			continue
		}
		for _, s := range r.contains {
			if !strings.Contains(c, s) {
				problems = append(problems, r.name+": comment lacks "+s)
			}
		}
		for _, s := range r.absent {
			if strings.Contains(c, s) {
				problems = append(problems, r.name+": comment must not contain "+s)
			}
		}
	}
	return problems
}

func reasonRule(name string, reasons ...string) docRule {
	r := docRule{name: name, kind: "rpc", contains: []string{"admin-only", "promotion.platform"}}
	r.contains = append(r.contains, reasons...)
	for _, a := range allReasons {
		found := false
		for _, x := range reasons {
			if x == a {
				found = true
			}
		}
		if !found {
			r.absent = append(r.absent, a)
		}
	}
	return r
}

func adminRules() []docRule {
	deact := reasonRule("DeactivateVoucher", "AUDIT_UNAVAILABLE", "PROMOTION_UNAVAILABLE")
	deact.contains = append(deact.contains, "voucher.deactivate")
	upd := reasonRule("UpdateVoucher", "AUDIT_UNAVAILABLE", "PROMOTION_UNAVAILABLE")
	upd.contains = append(upd.contains, "voucher.update")
	cv := reasonRule("CreateVoucher", "AUDIT_UNAVAILABLE", "PROMOTION_UNAVAILABLE")
	cv.contains = append(cv.contains, "ALREADY_EXISTS")
	cancel := reasonRule("CancelCampaign", "AUDIT_UNAVAILABLE", "PROMOTION_UNAVAILABLE")
	cancel.contains = append(cancel.contains, "campaign.cancel")
	list := reasonRule("ListCampaigns", "PROMOTION_UNAVAILABLE")
	cc := reasonRule("CreateCampaign", "AUDIT_UNAVAILABLE", "PROMOTION_UNAVAILABLE", "LISTING_UNAVAILABLE")
	cc.contains = append(cc.contains, "idempotency_key", "8..64", "[A-Za-z0-9_-]", "ALREADY_EXISTS")
	uc := reasonRule("UpdateCampaign", "AUDIT_UNAVAILABLE", "PROMOTION_UNAVAILABLE", "LISTING_UNAVAILABLE")
	uc.contains = append(uc.contains, "campaign.update")
	return []docRule{deact, upd, cv, cancel, list, cc, uc}
}

func identityRules() []docRule {
	return []docRule{
		{name: "CreateHandoffCode", kind: "rpc", contains: []string{"bearer", "never logged", "60 s", "single use", "RESOURCE_EXHAUSTED", "10"}},
		{name: "RedeemHandoffCode", kind: "rpc", contains: []string{"Public", "fixed UNAUTHENTICATED"}},
		{name: "PeekHandoffCode", kind: "rpc", contains: []string{"read-only", "does not consume", "fixed UNAUTHENTICATED"}},
		{name: "BecomeSeller", kind: "rpc", contains: []string{"Authenticated user"}},
		{name: "UserRoleGranted", kind: "message", contains: []string{"identity.events", "keyed by user id"}},
	}
}

func catalogueProblems(src string, actions []string) []string {
	c, ok := leadingComment(src, "message", "WriteAuditEventRequest")
	if !ok {
		return []string{"audit catalogue: WriteAuditEventRequest not found"}
	}
	var problems []string
	if !strings.Contains(c, "Action catalogue (24)") {
		problems = append(problems, "audit catalogue: header is not 'Action catalogue (24)'")
	}
	for _, a := range actions {
		if !strings.Contains(c, a) {
			problems = append(problems, "audit catalogue: lacks "+a)
		}
	}
	return problems
}

func mutateComment(t *testing.T, src, kind, name, old, repl string) string {
	t.Helper()
	lines := strings.Split(src, "\n")
	prefix := kind + " " + name
	for i, l := range lines {
		tl := strings.TrimSpace(l)
		if !strings.HasPrefix(tl, prefix+"(") && !strings.HasPrefix(tl, prefix+" ") {
			continue
		}
		changed := false
		for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "//"); j-- {
			if strings.Contains(lines[j], old) {
				lines[j] = strings.ReplaceAll(lines[j], old, repl)
				changed = true
			}
		}
		if !changed {
			t.Fatalf("%s: phrase %q not present in comment, cannot mutate", name, old)
		}
		return strings.Join(lines, "\n")
	}
	t.Fatalf("%s %s not found", kind, name)
	return ""
}

func requireProblemNaming(t *testing.T, problems []string, rpc string) {
	t.Helper()
	for _, p := range problems {
		if strings.HasPrefix(p, rpc+":") {
			return
		}
	}
	t.Errorf("expected a problem naming %s, got %v", rpc, problems)
}

const (
	promoPath    = "platform/promotion/v1/promotion.proto"
	identityPath = "platform/identity/v1/identity.proto"
	auditPath    = "platform/audit/v1/audit.proto"
)

// Scenarios: Comments name the authz rule and audit action; Missing authz comment is rejected
func TestGapsContract_AdminRPCsDocumented(t *testing.T) {
	promo := readSrc(t, promoPath)
	audit := readSrc(t, auditPath)
	rules := adminRules()

	t.Run("comments_name_authz_rule_and_audit_action", func(t *testing.T) {
		for _, p := range checkRules(promo, rules) {
			t.Error(p)
		}
		for _, p := range catalogueProblems(audit, []string{"voucher.deactivate", "voucher.update", "campaign.cancel", "campaign.update"}) {
			t.Error(p)
		}
	})

	if len(checkRules(promo, rules)) != 0 {
		t.Fatal("unmutated promotion.proto must be clean before rejection cases are meaningful")
	}

	t.Run("missing_authz_comment_is_rejected", func(t *testing.T) {
		cases := []struct{ rpc, old, repl string }{
			{"DeactivateVoucher", "admin-only", "restricted"},
			{"UpdateCampaign", "admin-only", "restricted"},
			{"ListCampaigns", "PROMOTION_UNAVAILABLE", "OTHER"},
			{"CancelCampaign", "AUDIT_UNAVAILABLE", "OTHER"},
			{"CreateCampaign", "idempotency_key", "key"},
			{"CreateCampaign", "LISTING_UNAVAILABLE", "OTHER"},
		}
		for _, c := range cases {
			mut := mutateComment(t, promo, "rpc", c.rpc, c.old, c.repl)
			tmp := filepath.Join(t.TempDir(), "promotion.proto")
			if err := os.WriteFile(tmp, []byte(mut), 0o600); err != nil {
				t.Fatal(err)
			}
			got := checkRules(readSrc(t, tmp), rules)
			requireProblemNaming(t, got, c.rpc)
		}
	})
}

// Scenarios: Comments carry the authz and secrecy rules; Missing secrecy comment is rejected
func TestGapsContract_IdentityRPCsDocumented(t *testing.T) {
	ident := readSrc(t, identityPath)
	audit := readSrc(t, auditPath)
	rules := identityRules()

	t.Run("comments_carry_authz_and_secrecy_rules", func(t *testing.T) {
		for _, p := range checkRules(ident, rules) {
			t.Error(p)
		}
		for _, p := range catalogueProblems(audit, []string{"auth.handoff_mint", "auth.handoff_redeem"}) {
			t.Error(p)
		}
	})

	t.Run("audit_catalogue_lists_24_with_six_new_actions", func(t *testing.T) {
		all := []string{"voucher.deactivate", "voucher.update", "campaign.cancel", "campaign.update", "auth.handoff_mint", "auth.handoff_redeem"}
		for _, p := range catalogueProblems(audit, all) {
			t.Error(p)
		}
	})

	if len(checkRules(ident, rules)) != 0 {
		t.Fatal("unmutated identity.proto must be clean before rejection cases are meaningful")
	}

	t.Run("missing_secrecy_comment_is_rejected", func(t *testing.T) {
		cases := []struct{ rpc, old, repl string }{
			{"CreateHandoffCode", "never logged", "kept quiet"},
			{"CreateHandoffCode", "60 s", "a while"},
			{"CreateHandoffCode", "single use", "reusable"},
			{"CreateHandoffCode", "RESOURCE_EXHAUSTED", "OTHER"},
			{"RedeemHandoffCode", "fixed UNAUTHENTICATED", "varied error"},
			{"PeekHandoffCode", "fixed UNAUTHENTICATED", "varied error"},
		}
		for _, c := range cases {
			mut := mutateComment(t, ident, "rpc", c.rpc, c.old, c.repl)
			tmp := filepath.Join(t.TempDir(), "identity.proto")
			if err := os.WriteFile(tmp, []byte(mut), 0o600); err != nil {
				t.Fatal(err)
			}
			requireProblemNaming(t, checkRules(readSrc(t, tmp), rules), c.rpc)
		}
	})
}
