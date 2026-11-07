package engagementv1_test

// Contract tests for change discovery-seo-contract (capability discovery-seo-contract): engagement events.proto.
// Baseline tables below are copied from origin/main at the time of the change.
// Helpers (tc*) come from trust_compat_test.go in this package.

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

var dsBaselineReviewEvents = map[string]map[string]string{
	"ReviewCreated": {
		"review_id": "1 string", "user_id": "2 string", "listing_id": "3 string", "seller_id": "4 string", "rating": "5 int32",
	},
	"ReviewHidden": {
		"review_id": "1 string", "listing_id": "2 string", "hidden_at": "3 " + engTS,
	},
}

// Scenario: Message shape is fixed
func TestDiscovery_ListingRatingChangedShape(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	tcShape(t, fd, "ListingRatingChanged", map[string]string{
		"listing_id": "1 string", "average_rating": "2 double", "review_count": "3 int64", "changed_at": "4 " + engTS,
	})
}

// Scenario: No reviewer or text fields
func TestDiscovery_ListingRatingChangedHasNoForbiddenFields(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	md := tcMsg(t, fd, "ListingRatingChanged")
	for _, f := range []string{"user_id", "reviewer_id", "review_id", "text", "title", "rating"} {
		if md.Fields().ByName(protoreflect.Name(f)) != nil {
			t.Errorf("ListingRatingChanged must not carry %s", f)
		}
	}
}

// Scenario: Last review hidden leaves zero
func TestDiscovery_ListingRatingChangedZeroReviews(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	md := tcMsg(t, fd, "ListingRatingChanged")
	m := tcNew(t, md, "listing_id", "l-1", "review_count", 0, "changed_at", tcTS(tcFld(t, md, "changed_at"), 1760000000))
	m.Set(tcFld(t, md, "average_rating"), protoreflect.ValueOfFloat64(0))
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, md, "average_rating")).Float() != 0 || got.Get(tcFld(t, md, "review_count")).Int() != 0 {
		t.Errorf("zero reviews must round trip as zero: %v", got)
	}
	tcCheck(t, "ListingRatingChanged.average_rating", tcFieldComment(t, fd, "ListingRatingChanged", "average_rating"), "0 when review_count is 0")
}

// Scenario: Topic, key and envelope type are documented
func TestDiscovery_ListingRatingChangedTopicKeyTypeDocumented(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	tcCheck(t, "ListingRatingChanged", tcComment(fd, tcMsg(t, fd, "ListingRatingChanged")),
		"engagement.events", "key = listing_id", "platform.engagement.v1.ListingRatingChanged")
}

// Scenario: Existing review events are untouched
func TestDiscovery_ReviewEventsUnchanged(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	for name, baseline := range dsBaselineReviewEvents {
		md := tcMsg(t, fd, name)
		for field, want := range baseline {
			f := md.Fields().ByName(protoreflect.Name(field))
			if f == nil {
				t.Errorf("%s.%s removed or renamed (baseline %s)", name, field, want)
				continue
			}
			if got := tcFmt(f); got != want {
				t.Errorf("%s.%s = %q, baseline %q", name, field, got, want)
			}
		}
	}
}

func tcFmt(f protoreflect.FieldDescriptor) string {
	return fmt.Sprintf("%d %s", f.Number(), tcKind(f))
}
