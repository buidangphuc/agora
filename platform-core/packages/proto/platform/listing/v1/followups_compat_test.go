package listingv1_test

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

var fuBaselineListingStatus = map[string]int{
	"LISTING_STATUS_UNSPECIFIED": 0, "LISTING_STATUS_DRAFT": 1, "LISTING_STATUS_PUBLISHED": 2,
	"LISTING_STATUS_REJECTED": 3, "LISTING_STATUS_PENDING_REVIEW": 4, "LISTING_STATUS_BANNED": 5,
	"LISTING_STATUS_HELD": 6,
}

func TestFollowups_ListingStatusDeletedIsSeven(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	want := map[string]int{"LISTING_STATUS_DELETED": 7}
	for k, v := range fuBaselineListingStatus {
		want[k] = v
	}
	tcEnum(t, fd, "ListingStatus", want)
}

func TestFollowups_ExistingStatusValuesUnchanged(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	ed := fd.Enums().ByName("ListingStatus")
	if ed == nil {
		t.Fatal("enum ListingStatus missing")
	}
	for name, num := range fuBaselineListingStatus {
		v := ed.Values().ByName(protoreflect.Name(name))
		if v == nil {
			t.Errorf("%s removed or renamed", name)
			continue
		}
		if int(v.Number()) != num {
			t.Errorf("%s = %d, baseline %d", name, v.Number(), num)
		}
	}
}

func TestFollowups_DeletedStatusDocumented(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	v := fd.Enums().ByName("ListingStatus").Values().ByName("LISTING_STATUS_DELETED")
	if v == nil {
		t.Fatal("LISTING_STATUS_DELETED missing")
	}
	tcCheck(t, "LISTING_STATUS_DELETED", tcComment(fd, v),
		"read models", "hard-deleted listing", "team-domain never returns it")
}

func TestFollowups_OldSchemaParsesDeleted(t *testing.T) {
	fd, files := tcFile(t, listingProto)
	if fd.Enums().ByName("ListingStatus").Values().ByNumber(7) == nil {
		t.Fatal("new schema lacks ListingStatus number 7")
	}
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropEnumValues(p, "ListingStatus", func(n int32) bool { return n <= 6 })
	})
	newMD, oldMD := tcMsg(t, fd, "Listing"), tcMsg(t, old, "Listing")
	m := tcNew(t, newMD, "id", "l-1", "title", "Shoes", "status", 7)
	got := tcParseWith(t, m, oldMD)
	if n := got.Get(tcFld(t, oldMD, "status")).Enum(); n != 7 {
		t.Errorf("old schema reads status = %d, want 7", n)
	}
	if old.Enums().ByName("ListingStatus").Values().ByNumber(7) != nil {
		t.Errorf("old schema must not define a name for 7")
	}
	if got.Get(tcFld(t, oldMD, "id")).String() != "l-1" || got.Get(tcFld(t, oldMD, "title")).String() != "Shoes" {
		t.Errorf("other fields changed: %v", got)
	}
}

func TestFollowups_NoUndefinedStatusNumbers(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	if v := fd.Enums().ByName("ListingStatus").Values().ByNumber(8); v != nil {
		t.Errorf("ListingStatus defines number 8 (%s); adding a value needs a spec change", v.Name())
	}
}
