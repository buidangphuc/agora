package listingv1_test

// Tests for change seller-centre-contract (listing half). Reuses the descriptor
// helpers of moderation_compat_test.go.

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const scStatusKind = "enum:platform.listing.v1.ListingStatus"

// Scenario: Seller centre contract: ListMyListings filter fields exist
func TestSellerCentre_ListMyListingsFilterFieldsExist(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	tcShape(t, fd, "ListMyListingsRequest", map[string]string{
		"page": "1 " + pageReq, "statuses": "2 repeated " + scStatusKind, "query": "3 string",
	})
	tcShape(t, fd, "ListMyListingsResponse", map[string]string{
		"listings": "1 repeated message:platform.listing.v1.Listing", "page": "2 " + pageResp,
	})
}

// Scenario: Seller centre contract: ListMyListings filter semantics are documented
func TestSellerCentre_ListMyListingsFilterSemanticsAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	md := tcMsg(t, fd, "ListMyListingsRequest")
	if md.Fields().ByName("statuses") == nil || md.Fields().ByName("query") == nil {
		t.Fatal("statuses/query missing")
	}
	all := tcComment(fd, md.Fields().ByName("statuses")) + " " + tcComment(fd, md.Fields().ByName("query")) + " " + tcComment(fd, md)
	tcCheck(t, "ListMyListingsRequest filter comments", all,
		"empty", "all", "case-insensitive", "substring", "100", "INVALID_ARGUMENT")
	tcCheck(t, "statuses comment", tcFieldComment(t, fd, "ListMyListingsRequest", "statuses"), "empty", "all")
	tcCheck(t, "query comment", tcFieldComment(t, fd, "ListMyListingsRequest", "query"),
		"case-insensitive", "substring", "100")
}

// Scenario: Seller centre contract: Empty filter request keeps the old wire form
func TestSellerCentre_EmptyFilterRequestKeepsTheOldWireForm(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	md := tcMsg(t, fd, "ListMyListingsRequest")
	page := tcNew(t, tcFld(t, md, "page").Message(), "page_size", 50)
	m := tcNew(t, md, "page", page)
	got, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x0a, 0x02, 0x10, 0x32}; !bytes.Equal(got, want) {
		t.Errorf("bytes = %x, want %x", got, want)
	}
}

// Scenario: Seller centre contract: Empty filter request keeps the old wire form
func TestSellerCentre_FilteredRequestRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	md := tcMsg(t, fd, "ListMyListingsRequest")
	sf, qf := tcFld(t, md, "statuses"), tcFld(t, md, "query")
	q := strings.Repeat("é", 100)
	m := dynamicpb.NewMessage(md)
	l := m.Mutable(sf).List()
	l.Append(protoreflect.ValueOfEnum(2))
	l.Append(protoreflect.ValueOfEnum(4))
	m.Set(qf, protoreflect.ValueOfString(q))
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(m, got) {
		t.Errorf("round trip differs: %v vs %v", m, got)
	}
	gl := got.Get(sf).List()
	if gl.Len() != 2 || gl.Get(0).Enum() != 2 || gl.Get(1).Enum() != 4 || got.Get(qf).String() != q {
		t.Errorf("decoded = %v", got)
	}
}

// Scenario: Seller centre contract: Storefront fields are unchanged
func TestSellerCentre_StorefrontFieldsAreUnchanged(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	tcShape(t, fd, "Storefront", map[string]string{
		"seller_id": "1 string", "slug": "2 string", "banner_url": "3 string", "tagline": "4 string",
		"featured_listing_ids": "5 repeated string", "theme": "6 string", "display_name": "7 string",
	})
	md := tcMsg(t, fd, "Storefront")
	for i := 0; i < md.Fields().Len(); i++ {
		if n := md.Fields().Get(i).Number(); n >= 8 {
			t.Errorf("Storefront has field number %d", n)
		}
	}
	tcShape(t, fd, "UpsertStorefrontRequest", map[string]string{"storefront": "1 message:platform.listing.v1.Storefront"})
}

// Scenario: Seller centre contract: Storefront rules are documented
func TestSellerCentre_StorefrontRulesAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	sm, um := tcMsg(t, fd, "Storefront"), tcMsg(t, fd, "UpsertStorefrontRequest")
	var parts []string
	for _, md := range []protoreflect.MessageDescriptor{sm, um} {
		parts = append(parts, tcComment(fd, md))
		for i := 0; i < md.Fields().Len(); i++ {
			parts = append(parts, tcComment(fd, md.Fields().Get(i)))
		}
	}
	tcCheck(t, "Storefront/UpsertStorefrontRequest comments", strings.Join(parts, " "),
		"120", "control", "listing-images", "GetImageUploadUrl", "6", "unique", "owned by the caller",
		"INVALID_ARGUMENT", "nothing is saved", "default", "dark", "festive", "full storefront")
}
