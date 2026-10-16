package listingv1_test

// Contract tests for change q6-trust-contract (capability trust-contract): listing moderation.
// Descriptors come from `buf build` (source info included); no generated code needed.

import (
	"bytes"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const listingProto = "platform/listing/v1/listing.proto"
const pageReq = "message:platform.common.v1.PageRequest"
const pageResp = "message:platform.common.v1.PageResponse"
const tsKind = "message:google.protobuf.Timestamp"

var listingStatusEnum = map[string]int{
	"LISTING_STATUS_UNSPECIFIED": 0, "LISTING_STATUS_DRAFT": 1, "LISTING_STATUS_PUBLISHED": 2,
	"LISTING_STATUS_REJECTED": 3, "LISTING_STATUS_PENDING_REVIEW": 4, "LISTING_STATUS_BANNED": 5,
	"LISTING_STATUS_HELD": 6,
}

// Scenario: Trust contract: New listing statuses have fixed numbers
func TestTrustContract_NewListingStatusesHaveFixedNumbers(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	tcEnum(t, fd, "ListingStatus", listingStatusEnum)
}

// Scenario: Trust contract: ModerationInfo fields are fixed
func TestTrustContract_ModerationInfoFieldsAreFixed(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	tcShape(t, fd, "ModerationInfo", map[string]string{
		"reason_code": "1 string", "note": "2 string", "screen_flags": "3 repeated string",
		"decided_at": "4 " + tsKind,
	})
	tcShape(t, fd, "Listing", map[string]string{
		"id": "1 string", "title": "2 string", "description": "3 string", "price": "4 int64",
		"currency": "5 string", "status": "6 enum:platform.listing.v1.ListingStatus",
		"seller_id": "7 string", "image_keys": "8 repeated string", "category_id": "9 string",
		"stock": "10 int32", "variants": "11 repeated message:platform.listing.v1.Variant",
		"moderation": "12 message:platform.listing.v1.ModerationInfo",
	})
}

func heldListing(t *testing.T, fd protoreflect.FileDescriptor) *dynamicpb.Message {
	t.Helper()
	listing, mi := tcMsg(t, fd, "Listing"), tcMsg(t, fd, "ModerationInfo")
	mod := tcNew(t, mi, "reason_code", "seller_enforcement", "note", "repeat offender",
		"screen_flags", []string{"weapon", "keyword:x"}, "decided_at", tcTS(tcFld(t, mi, "decided_at"), 1760000000))
	return tcNew(t, listing, "id", "l-1", "title", "t", "status", 6, "moderation", mod)
}

// Scenario: Trust contract: Listing with moderation round-trips
func TestTrustContract_ListingWithModerationRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	m := heldListing(t, fd)
	got := tcRoundEqual(t, m)
	listing := tcMsg(t, fd, "Listing")
	if got.Get(tcFld(t, listing, "status")).Enum() != 6 {
		t.Errorf("status = %v, want 6 (HELD)", got.Get(tcFld(t, listing, "status")).Enum())
	}
	mod := got.Get(tcFld(t, listing, "moderation")).Message()
	if mod.Get(mod.Descriptor().Fields().ByName("screen_flags")).List().Len() != 2 {
		t.Errorf("screen_flags lost in round trip")
	}
	// Without field 12 the parsed Listing has moderation unset.
	bare := tcRound(t, tcNew(t, listing, "id", "l-2", "status", 2))
	if bare.Has(tcFld(t, listing, "moderation")) {
		t.Errorf("moderation set on a Listing serialised without field 12")
	}
}

// Scenario: Trust contract: Old readers skip the moderation field and tolerate new statuses
func TestTrustContract_OldReadersSkipModerationAndTolerateNewStatuses(t *testing.T) {
	fd, files := tcFile(t, listingProto)
	m := heldListing(t, fd)
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropFields(p, "Listing", func(n int32) bool { return n <= 11 })
		tcDropEnumValues(p, "ListingStatus", func(n int32) bool { return n <= 3 })
	})
	oldListing := old.Messages().ByName("Listing")
	got := tcParseWith(t, m, oldListing)
	if got.Get(oldListing.Fields().ByName("status")).Enum() != 6 {
		t.Errorf("old reader lost unknown status 6")
	}
	if got.Get(oldListing.Fields().ByName("id")).String() != "l-1" {
		t.Errorf("old reader lost id")
	}
	if len(got.GetUnknown()) == 0 {
		t.Errorf("field 12 should be skipped into unknown fields by the old reader")
	}
}

// Scenario: Trust contract: Status is documented as server-owned
func TestTrustContract_StatusIsDocumentedAsServerOwned(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	listing, mi := tcMsg(t, fd, "Listing"), tcMsg(t, fd, "ModerationInfo")
	pool := tcComment(fd, tcFld(t, listing, "status")) + " " + tcComment(fd, tcFld(t, listing, "moderation")) +
		" " + tcComment(fd, mi)
	for i := 0; i < mi.Fields().Len(); i++ {
		pool += " " + tcComment(fd, mi.Fields().Get(i))
	}
	tcCheck(t, "Listing.status/moderation/ModerationInfo", pool,
		"server-owned", "ignored on writes", "DRAFT or publish", "keeps the stored status",
		"admin only", "reason_code and note", "ListingChanged", "no note", "no screen_flags",
		"prohibited_item", "counterfeit", "misleading", "inappropriate", "duplicate", "seller_enforcement", "other",
		"500 characters")
	tcCheck(t, "Listing.status", tcComment(fd, tcFld(t, listing, "status")), "server-owned")
}

// Scenario: Trust contract: Missing server-owned note is caught
func TestTrustContract_MissingServerOwnedNoteIsCaught(t *testing.T) {
	if err := tcKeys("Publication status of the listing.", "server-owned"); err == nil {
		t.Fatal("helper accepted a Listing.status comment lacking server-owned")
	}
	if err := tcKeys("", "server-owned"); err == nil {
		t.Fatal("helper accepted a missing comment")
	}
	if err := tcKeys("status is server-owned.", "server-owned"); err != nil {
		t.Fatalf("helper rejected a comment containing server-owned: %v", err)
	}
}

// Scenario: Trust contract: Seller cannot set server-owned fields
func TestTrustContract_SellerCannotSetServerOwnedFields(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	create := tcRPCComment(t, fd, "ListingService", "CreateListing")
	update := tcRPCComment(t, fd, "ListingService", "UpdateListing")
	del := tcRPCComment(t, fd, "ListingService", "DeleteListing")
	tcCheck(t, "CreateListing", create, "server-owned", "dropped", "no error")
	tcCheck(t, "UpdateListing", update, "server-owned", "dropped", "no error", "REJECTED", "PENDING_REVIEW", "never to PUBLISHED", "re-screened")
	tcCheck(t, "DeleteListing", del, "FAILED_PRECONDITION", "BANNED or HELD")
	tcCheck(t, "UpdateListing", update, "FAILED_PRECONDITION", "BANNED or HELD")
	pool := create + " " + update + " " + del
	tcCheck(t, "write RPCs", pool, "cannot publish past the screen", "undo a rejection",
		"admin writing their own listing follows the seller path", "still audited", "PERMISSION_DENIED")
}

var listingExisting = map[string][2]string{
	"GetListing": {"GetListingRequest", "GetListingResponse"}, "ListListings": {"ListListingsRequest", "ListListingsResponse"},
	"ListMyListings": {"ListMyListingsRequest", "ListMyListingsResponse"}, "CreateListing": {"CreateListingRequest", "CreateListingResponse"},
	"UpdateListing": {"UpdateListingRequest", "UpdateListingResponse"}, "DeleteListing": {"DeleteListingRequest", "DeleteListingResponse"},
	"GetImageUploadUrl": {"GetImageUploadUrlRequest", "GetImageUploadUrlResponse"},
	"ListCategories":    {"ListCategoriesRequest", "ListCategoriesResponse"}, "GetCategory": {"GetCategoryRequest", "GetCategoryResponse"},
	"ReserveStock": {"ReserveStockRequest", "ReserveStockResponse"}, "ReleaseStock": {"ReleaseStockRequest", "ReleaseStockResponse"},
	"CommitReservation": {"CommitReservationRequest", "CommitReservationResponse"},
	"UpsertStorefront":  {"UpsertStorefrontRequest", "UpsertStorefrontResponse"}, "GetStorefront": {"GetStorefrontRequest", "GetStorefrontResponse"},
	"BatchGetStorefronts": {"BatchGetStorefrontsRequest", "BatchGetStorefrontsResponse"},
	"CreateBundle":        {"CreateBundleRequest", "CreateBundleResponse"}, "GetBundle": {"GetBundleRequest", "GetBundleResponse"},
	"ListBundlesBySeller": {"ListBundlesBySellerRequest", "ListBundlesBySellerResponse"},
}

// Scenario: Trust contract: Moderation RPCs exist on ListingService
func TestTrustContract_ModerationRPCsExistOnListingService(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	want := map[string][2]string{
		"ModerateListing":     {"ModerateListingRequest", "ModerateListingResponse"},
		"ListModerationQueue": {"ListModerationQueueRequest", "ListModerationQueueResponse"},
	}
	for k, v := range listingExisting {
		want[k] = v
	}
	tcRPCs(t, fd, "ListingService", want)
}

// Scenario: Trust contract: Moderation messages have fixed numbers
func TestTrustContract_ModerationMessagesHaveFixedNumbers(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	tcEnum(t, fd, "ModerationDecision", map[string]int{
		"MODERATION_DECISION_UNSPECIFIED": 0, "MODERATION_DECISION_APPROVE": 1,
		"MODERATION_DECISION_REJECT": 2, "MODERATION_DECISION_BAN": 3,
	})
	tcShape(t, fd, "ModerateListingRequest", map[string]string{
		"listing_id": "1 string", "decision": "2 enum:platform.listing.v1.ModerationDecision",
		"reason_code": "3 string", "note": "4 string",
	})
	tcShape(t, fd, "ModerateListingResponse", map[string]string{"listing": "1 message:platform.listing.v1.Listing"})
	tcShape(t, fd, "ListModerationQueueRequest", map[string]string{"page": "1 " + pageReq})
	tcShape(t, fd, "ModerationQueueItem", map[string]string{
		"listing": "1 message:platform.listing.v1.Listing", "submitted_at": "2 " + tsKind,
	})
	tcShape(t, fd, "ListModerationQueueResponse", map[string]string{
		"items": "1 repeated message:platform.listing.v1.ModerationQueueItem", "page": "2 " + pageResp,
	})
}

// Scenario: Trust contract: Moderation request round-trips
func TestTrustContract_ModerationRequestRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	req := tcMsg(t, fd, "ModerateListingRequest")
	note := strings.Repeat("n", 500)
	m := tcNew(t, req, "listing_id", "l-1", "decision", 3, "reason_code", "counterfeit", "note", note)
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, req, "note")).String() != note || got.Get(tcFld(t, req, "decision")).Enum() != 3 {
		t.Errorf("note or decision changed in round trip")
	}
}

// Scenario: Trust contract: Moderation authz and audit are documented
func TestTrustContract_ModerationAuthzAndAuditAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	mod := tcRPCComment(t, fd, "ListingService", "ModerateListing")
	q := tcRPCComment(t, fd, "ListingService", "ListModerationQueue")
	for name, c := range map[string]string{"ModerateListing": mod, "ListModerationQueue": q} {
		tcCheck(t, name, c, "admin-only", "scope admin", "UNAUTHENTICATED", "PERMISSION_DENIED")
	}
	tcCheck(t, "ModerateListing", mod, "listing.moderate", "first", "UNAVAILABLE", "no change")
	tcCheck(t, "ListModerationQueue", q, "oldest", "not audited")
}

// Scenario: Trust contract: Moderation errors and transitions are documented
func TestTrustContract_ModerationErrorsAndTransitionsAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, listingProto)
	mod := tcRPCComment(t, fd, "ListingService", "ModerateListing")
	tcCheck(t, "ModerateListing", mod, "INVALID_ARGUMENT", "empty listing_id", "UNSPECIFIED decision",
		"reason_code", "note over 500", "NOT_FOUND", "FAILED_PRECONDITION",
		"APPROVE from PENDING_REVIEW only", "REJECT and BAN from PENDING_REVIEW or PUBLISHED",
		"nothing from DRAFT, HELD, REJECTED or BANNED", "ListingChanged(UPDATED)", "without the note or screen flags")
}

// ---- shared helpers (prefix tc; descriptors come from `buf build`, no generated code) ----

func tcFile(t *testing.T, path string) (protoreflect.FileDescriptor, *protoregistry.Files) {
	t.Helper()
	cmd := exec.Command("buf", "build", "-o", "-#format=binpb")
	cmd.Dir = "../../.." // packages/proto (buf module root)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed (harness): %v\n%s", err, stderr.String())
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(out.Bytes(), &set); err != nil {
		t.Fatalf("decode image: %v", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		t.Fatalf("build files: %v", err)
	}
	fd, err := files.FindFileByPath(path)
	if err != nil {
		t.Fatalf("find %s: %v", path, err)
	}
	return fd, files
}

func tcMsg(t *testing.T, fd protoreflect.FileDescriptor, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(name))
	if md == nil {
		t.Fatalf("message %s missing from %s", name, fd.Path())
	}
	return md
}

func tcFld(t *testing.T, md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	t.Helper()
	f := md.Fields().ByName(protoreflect.Name(name))
	if f == nil {
		t.Fatalf("%s.%s missing", md.Name(), name)
	}
	return f
}

func tcKind(f protoreflect.FieldDescriptor) string {
	var s string
	switch {
	case f.IsMap():
		s = "map"
	case f.Kind() == protoreflect.MessageKind || f.Kind() == protoreflect.GroupKind:
		s = "message:" + string(f.Message().FullName())
	case f.Kind() == protoreflect.EnumKind:
		s = "enum:" + string(f.Enum().FullName())
	default:
		s = f.Kind().String()
	}
	if f.IsList() {
		s = "repeated " + s
	}
	return s
}

// tcShape asserts the message has exactly the wanted fields: name -> "<number> <kind>".
// A wanted value may list alternatives separated by "|" (each "<number> <kind>").
func tcShape(t *testing.T, fd protoreflect.FileDescriptor, msgName string, want map[string]string) {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(msgName))
	if md == nil {
		t.Errorf("message %s missing", msgName)
		return
	}
	got := map[string]string{}
	for i := 0; i < md.Fields().Len(); i++ {
		f := md.Fields().Get(i)
		got[string(f.Name())] = fmt.Sprintf("%d %s", f.Number(), tcKind(f))
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s.%s missing (want %s)", msgName, name, w)
			continue
		}
		match := false
		for _, alt := range strings.Split(w, "|") {
			if g == alt {
				match = true
			}
		}
		if !match {
			t.Errorf("%s.%s = %q, want %q", msgName, name, g, w)
		}
	}
	for name, g := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s has unexpected field %s = %q", msgName, name, g)
		}
	}
}

func tcEnum(t *testing.T, fd protoreflect.FileDescriptor, name string, want map[string]int) {
	t.Helper()
	ed := fd.Enums().ByName(protoreflect.Name(name))
	if ed == nil {
		t.Errorf("enum %s missing", name)
		return
	}
	got := map[string]int{}
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		got[string(v.Name())] = int(v.Number())
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enum %s = %v, want exactly %v", name, got, want)
	}
}

// tcRPCs asserts the service has exactly the wanted methods: name -> {request, response} short names.
func tcRPCs(t *testing.T, fd protoreflect.FileDescriptor, svc string, want map[string][2]string) {
	t.Helper()
	sd := fd.Services().ByName(protoreflect.Name(svc))
	if sd == nil {
		t.Errorf("service %s missing", svc)
		return
	}
	got := map[string][2]string{}
	for i := 0; i < sd.Methods().Len(); i++ {
		m := sd.Methods().Get(i)
		got[string(m.Name())] = [2]string{string(m.Input().Name()), string(m.Output().Name())}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s methods = %v, want exactly %v", svc, got, want)
	}
}

func tcNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

func tcComment(fd protoreflect.FileDescriptor, d protoreflect.Descriptor) string {
	loc := fd.SourceLocations().ByDescriptor(d)
	return tcNorm(loc.LeadingComments + " " + loc.TrailingComments)
}

func tcRPCComment(t *testing.T, fd protoreflect.FileDescriptor, svc, rpc string) string {
	t.Helper()
	sd := fd.Services().ByName(protoreflect.Name(svc))
	if sd == nil {
		t.Errorf("service %s missing", svc)
		return ""
	}
	m := sd.Methods().ByName(protoreflect.Name(rpc))
	if m == nil {
		t.Errorf("rpc %s.%s missing", svc, rpc)
		return ""
	}
	return tcComment(fd, m)
}

func tcFieldComment(t *testing.T, fd protoreflect.FileDescriptor, msg, field string) string {
	t.Helper()
	return tcComment(fd, tcFld(t, tcMsg(t, fd, msg), field))
}

// tcKeys returns an error naming every key (whitespace-normalised substring) missing from the comment.
func tcKeys(comment string, keys ...string) error {
	c := tcNorm(comment)
	if c == "" {
		return fmt.Errorf("no comment found on element")
	}
	var missing []string
	for _, k := range keys {
		if !strings.Contains(c, tcNorm(k)) {
			missing = append(missing, fmt.Sprintf("%q", k))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("comment missing %s; got:\n%s", strings.Join(missing, ", "), c)
	}
	return nil
}

func tcCheck(t *testing.T, what, comment string, keys ...string) {
	t.Helper()
	if err := tcKeys(comment, keys...); err != nil {
		t.Errorf("%s: %v", what, err)
	}
}

// tcOld builds a copy of the file as an old reader knew it (mutate edits the descriptor proto).
func tcOld(t *testing.T, files *protoregistry.Files, fd protoreflect.FileDescriptor, mutate func(*descriptorpb.FileDescriptorProto)) protoreflect.FileDescriptor {
	t.Helper()
	p := protodesc.ToFileDescriptorProto(fd)
	mutate(p)
	old, err := protodesc.NewFile(p, files)
	if err != nil {
		t.Fatalf("build old-schema file: %v", err)
	}
	return old
}

func tcDropFields(p *descriptorpb.FileDescriptorProto, msg string, keep func(num int32) bool) {
	for _, m := range p.MessageType {
		if m.GetName() != msg {
			continue
		}
		var fs []*descriptorpb.FieldDescriptorProto
		for _, f := range m.Field {
			if keep(f.GetNumber()) {
				fs = append(fs, f)
			}
		}
		m.Field = fs
	}
}

func tcDropEnumValues(p *descriptorpb.FileDescriptorProto, enum string, keep func(num int32) bool) {
	for _, e := range p.EnumType {
		if e.GetName() != enum {
			continue
		}
		var vs []*descriptorpb.EnumValueDescriptorProto
		for _, v := range e.Value {
			if keep(v.GetNumber()) {
				vs = append(vs, v)
			}
		}
		e.Value = vs
	}
}

func tcNew(t *testing.T, md protoreflect.MessageDescriptor, kv ...any) *dynamicpb.Message {
	t.Helper()
	m := dynamicpb.NewMessage(md)
	for i := 0; i < len(kv); i += 2 {
		f := md.Fields().ByName(protoreflect.Name(kv[i].(string)))
		if f == nil {
			t.Fatalf("no field %s on %s", kv[i].(string), md.Name())
		}
		switch v := kv[i+1].(type) {
		case string:
			m.Set(f, protoreflect.ValueOfString(v))
		case int:
			switch f.Kind() {
			case protoreflect.EnumKind:
				m.Set(f, protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)))
			case protoreflect.Int32Kind:
				m.Set(f, protoreflect.ValueOfInt32(int32(v)))
			default:
				m.Set(f, protoreflect.ValueOfInt64(int64(v)))
			}
		case []string:
			l := m.Mutable(f).List()
			for _, s := range v {
				l.Append(protoreflect.ValueOfString(s))
			}
		case *dynamicpb.Message:
			m.Set(f, protoreflect.ValueOfMessage(v))
		default:
			panic("unsupported value type")
		}
	}
	return m
}

// tcTS builds a Timestamp for the given message field.
func tcTS(f protoreflect.FieldDescriptor, seconds int64) *dynamicpb.Message {
	ts := dynamicpb.NewMessage(f.Message())
	ts.Set(f.Message().Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	return ts
}

// tcRound serialises m and parses it back into a fresh message of the same descriptor.
func tcRound(t *testing.T, m *dynamicpb.Message) *dynamicpb.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(m.Descriptor())
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatal(err)
	}
	return got
}

func tcRoundEqual(t *testing.T, m *dynamicpb.Message) *dynamicpb.Message {
	t.Helper()
	got := tcRound(t, m)
	if !proto.Equal(got, m) {
		t.Errorf("%s changed in round trip:\n got %v\nwant %v", m.Descriptor().Name(), got, m)
	}
	return got
}

// tcParseWith serialises m (new schema) and parses it with the old message descriptor.
func tcParseWith(t *testing.T, m *dynamicpb.Message, old protoreflect.MessageDescriptor) *dynamicpb.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(old)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("old reader failed to parse: %v", err)
	}
	return got
}
