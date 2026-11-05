package edge_test

import (
	"net/http"
	"strings"
	"testing"
)

type promotionAdminCall struct {
	name, path, body string
	voucher          bool
}

var promotionAdminCalls = []promotionAdminCall{
	{"DeactivateVoucher", deactivateVoucherPath, `{"voucher_id":"v-1"}`, true},
	{"UpdateVoucher", updateVoucherPath, `{"voucher_id":"v-1","quota":150}`, true},
	{"ListCampaigns", listCampaignsPath, `{}`, false},
	{"CancelCampaign", cancelCampaignPath, `{"campaign_id":"c-1"}`, false},
	{"UpdateCampaign", updateCampaignPath, `{"campaign_id":"c-1","sale_price":900}`, false},
}

func callPromotionAdmin(t *testing.T, c promotionAdminCall, scopes []string) (int, string, int64) {
	t.Helper()
	if c.voucher {
		up := &voucherAdminUpstream{}
		code, body := callVoucherAdmin(t, up, c.path, c.body, scopes)
		return code, body, up.total()
	}
	up := &campaignAdminUpstream{}
	code, body := callCampaign(t, up, c.path, c.body, scopes)
	return code, body, up.total()
}

func TestPromotionAdminProceduresNeedPromotionAdmin(t *testing.T) {
	for _, c := range promotionAdminCalls {
		code, body, calls := callPromotionAdmin(t, c, []string{"listing.read", "listing.write", "admin", "order.admin"})
		if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
			t.Errorf("%s admin without promotion.admin = %d %s, want 403 permission_denied", c.name, code, body)
		}
		if calls != 0 {
			t.Errorf("%s admin without promotion.admin reached promotion %d times, want 0", c.name, calls)
		}
	}
}

func TestPromotionAdminProceduresForwardPromotionAdmin(t *testing.T) {
	for _, c := range promotionAdminCalls {
		code, body, calls := callPromotionAdmin(t, c, []string{"admin", "promotion.admin"})
		if code != http.StatusOK {
			t.Errorf("%s admin with promotion.admin = %d %s, want 200", c.name, code, body)
		}
		if calls != 1 {
			t.Errorf("%s admin with promotion.admin reached promotion %d times, want 1", c.name, calls)
		}
	}
}

func TestPromotionAdminProceduresAnonymousUnauthenticated(t *testing.T) {
	for _, c := range promotionAdminCalls {
		code, body, calls := callPromotionAdmin(t, c, nil)
		if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
			t.Errorf("%s anonymous = %d %s, want 401 unauthenticated", c.name, code, body)
		}
		if calls != 0 {
			t.Errorf("%s anonymous reached promotion %d times, want 0", c.name, calls)
		}
	}
}
