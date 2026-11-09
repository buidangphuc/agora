package engagementv1_test

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

func TestDiscovery_ListingRatingChangedShape(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	tcShape(t, fd, "ListingRatingChanged", map[string]string{
		"listing_id": "1 string", "average_rating": "2 double", "review_count": "3 int64", "changed_at": "4 " + engTS,
	})
}

func TestDiscovery_ListingRatingChangedHasNoForbiddenFields(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	md := tcMsg(t, fd, "ListingRatingChanged")
	for _, f := range []string{"user_id", "reviewer_id", "review_id", "text", "title", "rating"} {
		if md.Fields().ByName(protoreflect.Name(f)) != nil {
			t.Errorf("ListingRatingChanged must not carry %s", f)
		}
	}
}

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

func TestDiscovery_ListingRatingChangedTopicKeyTypeDocumented(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	tcCheck(t, "ListingRatingChanged", tcComment(fd, tcMsg(t, fd, "ListingRatingChanged")),
		"engagement.events", "key = listing_id", "platform.engagement.v1.ListingRatingChanged")
}

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
