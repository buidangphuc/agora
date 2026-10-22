package engagementv1_test

// Contract tests for change pah-contract (capability hardening-contract): the ReviewHidden event.

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Scenario: Hardening contract: ReviewHidden carries only the agreed fields
func TestHardeningContract_ReviewHiddenMessageIsFixed(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	tcShape(t, fd, "ReviewHidden", map[string]string{
		"review_id": "1 string", "listing_id": "2 string", "hidden_at": "3 " + engTS,
	})
	rh := tcMsg(t, fd, "ReviewHidden")
	for _, forbidden := range []string{"user_id", "hidden_by", "hide_reason", "text", "title"} {
		if rh.Fields().ByName(protoreflect.Name(forbidden)) != nil {
			t.Errorf("ReviewHidden must not carry %s", forbidden)
		}
	}
	m := tcNew(t, rh, "review_id", "r-1", "listing_id", "l-1", "hidden_at", tcTS(tcFld(t, rh, "hidden_at"), 1760000000))
	tcRoundEqual(t, m)
}
