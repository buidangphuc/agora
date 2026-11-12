package searchv1_test

import (
	"bytes"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

var fuNewMessages = map[string]bool{"GetListingStockRequest": true, "ListingStock": true, "GetListingStockResponse": true}

var fuBaselineRPCs = map[string][2]string{
	"SearchListings":    {"SearchListingsRequest", "SearchListingsResponse"},
	"Suggest":           {"SuggestRequest", "SuggestResponse"},
	"SaveSearch":        {"SaveSearchRequest", "SaveSearchResponse"},
	"ListSavedSearches": {"ListSavedSearchesRequest", "ListSavedSearchesResponse"},
	"DeleteSavedSearch": {"DeleteSavedSearchRequest", "DeleteSavedSearchResponse"},
	"RunSavedSearch":    {"RunSavedSearchRequest", "RunSavedSearchResponse"},
}

var fuBaselineMessages = map[string]map[string]string{
	"SearchListingsRequest": {
		"query": "1 string", "page": "2 message:platform.common.v1.PageRequest", "filters": "3 map",
		"category_id": "4 string", "min_price": "5 int64", "max_price": "6 int64",
		"sort_by": "7 enum:platform.search.v1.SortBy", "min_rating": "8 int32",
		"search_mode": "9 enum:platform.search.v1.SearchMode",
	},
	"SearchHit": {
		"listing_id": "1 string", "score": "2 float", "stock": "3 optional int32", "sold_count": "4 int64",
		"rating_avg": "5 double", "review_count": "6 int64", "seller_id": "7 string",
	},
	"SearchListingsResponse": {
		"hits": "1 repeated message:platform.search.v1.SearchHit", "page": "2 message:platform.common.v1.PageResponse",
		"facets": "3 message:platform.search.v1.Facets", "request_id": "4 string", "model_version": "5 string",
	},
	"FacetBucket":    {"key": "1 string", "count": "2 int64"},
	"AttributeFacet": {"group": "1 string", "buckets": "2 repeated message:platform.search.v1.FacetBucket"},
	"Facets": {
		"categories":   "1 repeated message:platform.search.v1.FacetBucket",
		"price_ranges": "2 repeated message:platform.search.v1.FacetBucket",
		"ratings":      "3 repeated message:platform.search.v1.FacetBucket",
		"sellers":      "4 repeated message:platform.search.v1.FacetBucket",
		"tags":         "5 repeated message:platform.search.v1.AttributeFacet",
		"skus":         "6 repeated message:platform.search.v1.AttributeFacet",
	},
	"SuggestRequest":  {"query": "1 string", "limit": "2 int32"},
	"SuggestResponse": {"suggestions": "1 repeated string"},
	"SavedSearch": {
		"id": "1 string", "query": "2 string", "filters_json": "3 string", "created_at": "4 " + dsTS,
	},
	"SaveSearchRequest":        {"query": "1 string", "filters_json": "2 string"},
	"SaveSearchResponse":       {"saved_search": "1 message:platform.search.v1.SavedSearch"},
	"ListSavedSearchesRequest": {"page": "1 message:platform.common.v1.PageRequest"},
	"ListSavedSearchesResponse": {
		"saved_searches": "1 repeated message:platform.search.v1.SavedSearch",
		"page":           "2 message:platform.common.v1.PageResponse",
	},
	"DeleteSavedSearchRequest":  {"id": "1 string"},
	"DeleteSavedSearchResponse": {},
	"RunSavedSearchRequest":     {"id": "1 string"},
	"RunSavedSearchResponse": {
		"hits": "1 repeated message:platform.search.v1.SearchHit", "page": "2 message:platform.common.v1.PageResponse",
		"facets": "3 message:platform.search.v1.Facets", "request_id": "4 string", "model_version": "5 string",
	},
}

func fuOldSearch(t *testing.T) (protoreflect.FileDescriptor, protoreflect.FileDescriptor) {
	t.Helper()
	fd, files := dsFile(t, dsSearchProto)
	old := dsOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		var ms []*descriptorpb.DescriptorProto
		for _, m := range p.MessageType {
			if !fuNewMessages[m.GetName()] {
				ms = append(ms, m)
			}
		}
		p.MessageType = ms
		for _, s := range p.Service {
			var methods []*descriptorpb.MethodDescriptorProto
			for _, m := range s.Method {
				if m.GetName() != "GetListingStock" {
					methods = append(methods, m)
				}
			}
			s.Method = methods
		}
	})
	return fd, old
}

func fuRPCComment(t *testing.T, fd protoreflect.FileDescriptor) string {
	t.Helper()
	m := fd.Services().ByName("SearchService").Methods().ByName("GetListingStock")
	if m == nil {
		t.Errorf("rpc GetListingStock missing")
		return ""
	}
	return dsComment(fd, m)
}

func TestFollowups_GetListingStockShape(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	m := fd.Services().ByName("SearchService").Methods().ByName("GetListingStock")
	if m == nil {
		t.Fatalf("SearchService.GetListingStock missing")
	}
	if m.Input().Name() != "GetListingStockRequest" || m.Output().Name() != "GetListingStockResponse" {
		t.Errorf("GetListingStock = %s -> %s", m.Input().Name(), m.Output().Name())
	}
	if m.IsStreamingClient() || m.IsStreamingServer() {
		t.Errorf("GetListingStock must be unary")
	}
	dsShape(t, fd, "GetListingStockRequest", map[string]string{"listing_ids": "1 repeated string"})
	dsShape(t, fd, "ListingStock", map[string]string{"listing_id": "1 string", "stock": "2 optional int32"})
	dsShape(t, fd, "GetListingStockResponse", map[string]string{"listings": "1 repeated message:platform.search.v1.ListingStock"})
}

func TestFollowups_ExistingSearchSurfaceUntouched(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	sd := fd.Services().ByName("SearchService")
	got := map[string][2]string{}
	for i := 0; i < sd.Methods().Len(); i++ {
		m := sd.Methods().Get(i)
		got[string(m.Name())] = [2]string{string(m.Input().Name()), string(m.Output().Name())}
	}
	for name, want := range fuBaselineRPCs {
		if g, ok := got[name]; !ok || !reflect.DeepEqual(g, want) {
			t.Errorf("rpc %s = %v (present=%v), baseline %v", name, g, ok, want)
		}
	}
	if len(got) != len(fuBaselineRPCs)+1 {
		t.Errorf("SearchService has %d rpcs, want baseline %d plus GetListingStock", len(got), len(fuBaselineRPCs))
	}
	for msg, fields := range fuBaselineMessages {
		dsShape(t, fd, msg, fields)
	}
	if n := fd.Messages().Len(); n != len(fuBaselineMessages)+len(fuNewMessages) {
		t.Errorf("file has %d messages, want %d baseline + %d new", n, len(fuBaselineMessages), len(fuNewMessages))
	}
}

func TestFollowups_StockPresenceRoundTrip(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	md := dsMsg(t, fd, "ListingStock")
	zero := dsNew(t, md, "listing_id", "l-1", "stock", int32(0))
	unset := dsNew(t, md, "listing_id", "l-1")
	stock := dsFld(t, md, "stock")
	gz := dsRoundEqual(t, zero)
	if !gz.Has(stock) || gz.Get(stock).Int() != 0 {
		t.Errorf("zero stock lost presence: %v", gz)
	}
	gu := dsRoundEqual(t, unset)
	if gu.Has(stock) {
		t.Errorf("unset stock gained presence: %v", gu)
	}
	bz, err := proto.Marshal(zero)
	if err != nil {
		t.Fatal(err)
	}
	bu, err := proto.Marshal(unset)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(bz, bu) {
		t.Errorf("zero stock and absent stock serialise identically: %x", bz)
	}
}

func TestFollowups_RequestRoundTrip(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	md := dsMsg(t, fd, "GetListingStockRequest")
	ids := []string{"l-1", "l-2", "l-3", "l-4", "l-5", "l-6", "l-7", "l-8", "l-9", "l-10"}
	got := dsRoundEqual(t, dsNew(t, md, "listing_ids", ids))
	l := got.Get(dsFld(t, md, "listing_ids")).List()
	if l.Len() != 10 {
		t.Fatalf("got %d ids, want 10", l.Len())
	}
	for i, id := range ids {
		if l.Get(i).String() != id {
			t.Errorf("id %d = %s, want %s", i, l.Get(i).String(), id)
		}
	}
	empty := dsParse(t, dynamicpb.NewMessage(md), md)
	if empty.Get(dsFld(t, md, "listing_ids")).List().Len() != 0 {
		t.Errorf("empty request must read an empty list: %v", empty)
	}
}

func TestFollowups_BoundAndOrderingDocumented(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	dsCheck(t, "GetListingStockRequest.listing_ids", dsFieldComment(t, fd, "GetListingStockRequest", "listing_ids"),
		"1..10", "INVALID_ARGUMENT")
	dsCheck(t, "GetListingStockResponse.listings", dsFieldComment(t, fd, "GetListingStockResponse", "listings"),
		"one entry per requested id", "request order")
}

func TestFollowups_StockSemanticsDocumented(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	dsCheck(t, "ListingStock.stock", dsFieldComment(t, fd, "ListingStock", "stock"),
		"projected base stock", "net of reservations", "greater than 0", "in_stock", "SearchHit.stock",
		"no variant rollup", "absent", "not projected or unknown")
	dsCheck(t, "rpc GetListingStock", fuRPCComment(t, fd), "service-to-service only")
}

func TestFollowups_WireCompatible(t *testing.T) {
	fd, old := fuOldSearch(t)
	if old.Messages().ByName("ListingStock") != nil || old.Messages().ByName("GetListingStockRequest") != nil {
		t.Fatalf("old schema must lack the new messages")
	}
	if fd.Messages().ByName("ListingStock") == nil {
		t.Fatalf("new schema lacks ListingStock")
	}
	for _, r := range dsResponses {
		newMD, oldMD := dsMsg(t, fd, r), dsMsg(t, old, r)
		oldHit := dsNew(t, dsMsg(t, old, "SearchHit"), "listing_id", "l-1", "stock", int32(3), "seller_id", "s-1")
		oldMsg := dynamicpb.NewMessage(oldMD)
		oldMsg.Mutable(dsFld(t, oldMD, "hits")).List().Append(protoreflect.ValueOfMessage(oldHit))
		fromOld := dsParse(t, oldMsg, newMD)
		hits := fromOld.Get(dsFld(t, newMD, "hits")).List()
		if hits.Len() != 1 {
			t.Fatalf("%s: old payload hits lost: %v", r, fromOld)
		}
		h := hits.Get(0).Message()
		if dsStr(h, "listing_id") != "l-1" || dsInt(h, "stock") != 3 || dsStr(h, "seller_id") != "s-1" {
			t.Errorf("%s: old hit fields changed: %v", r, h)
		}
	}
	stock := dsNew(t, dsMsg(t, fd, "ListingStock"), "listing_id", "l-1", "stock", int32(0))
	b, err := proto.Marshal(stock)
	if err != nil {
		t.Fatal(err)
	}
	resp := dsNew(t, dsMsg(t, fd, "GetListingStockResponse"))
	resp.Mutable(dsFld(t, resp.Descriptor(), "listings")).List().Append(protoreflect.ValueOfMessage(stock))
	rb, err := proto.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string][]byte{"ListingStock": b, "GetListingStockResponse": rb} {
		into := dynamicpb.NewMessage(dsMsg(t, old, "SuggestResponse"))
		if err := proto.Unmarshal(payload, into); err != nil {
			t.Errorf("%s bytes failed to parse under a schema lacking the new messages: %v", name, err)
		}
	}
}

func TestFollowups_NoPersonalFields(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	want := map[string]map[string]string{
		"GetListingStockRequest":  {"listing_ids": "1 repeated string"},
		"ListingStock":            {"listing_id": "1 string", "stock": "2 optional int32"},
		"GetListingStockResponse": {"listings": "1 repeated message:platform.search.v1.ListingStock"},
	}
	for msg, fields := range want {
		dsShape(t, fd, msg, fields)
		if md := fd.Messages().ByName(protoreflect.Name(msg)); md != nil {
			dsNoFields(t, md, "user_id", "seller_id", "buyer_id", "email", "phone")
		}
	}
}
