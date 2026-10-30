package upstream

import (
	"testing"
	"time"
)

// Task 1.6: Dial takes []Addr (name, address) and builds every service client.
func TestDialBuildsEveryClientFromAddrs(t *testing.T) {
	names := []string{"search", "listing", "identity", "engagement", "order", "payment", "chat", "ai", "recommendation", "promotion", "notification", "analytics", "referral", "verification", "sharing", "audit"}
	addrs := make([]Addr, 0, len(names))
	for _, n := range names {
		addrs = append(addrs, Addr{Name: n, Address: "127.0.0.1:1"})
	}
	c, err := Dial(time.Second, addrs)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	for name, ok := range map[string]bool{
		"Search": c.Search != nil, "Listing": c.Listing != nil, "Identity": c.Identity != nil,
		"Engagement": c.Engagement != nil, "Order": c.Order != nil, "Payment": c.Payment != nil,
		"Chat": c.Chat != nil, "AI": c.AI != nil, "Recommendation": c.Recommendation != nil,
		"Voucher": c.Voucher != nil, "Notification": c.Notification != nil,
		"AnalyticsQuery": c.AnalyticsQuery != nil, "Referral": c.Referral != nil,
		"Verification": c.Verification != nil, "Sharing": c.Sharing != nil, "Audit": c.Audit != nil,
	} {
		if !ok {
			t.Errorf("client %s not built", name)
		}
	}
}
