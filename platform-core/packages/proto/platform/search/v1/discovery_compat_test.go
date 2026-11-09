package searchv1_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const dsSearchProto = "platform/search/v1/search.proto"

var dsBaselineSortBy = map[string]int32{
	"SORT_BY_UNSPECIFIED": 0, "SORT_BY_RELEVANCE": 1, "SORT_BY_PRICE_ASC": 2, "SORT_BY_PRICE_DESC": 3, "SORT_BY_NEWEST": 4,
}

var dsBaselineHit = map[string]string{
	"listing_id": "1 string", "score": "2 float", "stock": "3 optional int32",
}

var dsBaselineResponse = map[string]string{
	"hits":   "1 repeated message:platform.search.v1.SearchHit",
	"page":   "2 message:platform.common.v1.PageResponse",
	"facets": "3 message:platform.search.v1.Facets",
}

var dsResponses = []string{"SearchListingsResponse", "RunSavedSearchResponse"}

func dsOldSearch(t *testing.T) (protoreflect.FileDescriptor, protoreflect.FileDescriptor) {
	t.Helper()
	fd, files := dsFile(t, dsSearchProto)
	old := dsOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		dsDropFields(p, "SearchHit", func(n int32) bool { return n <= 3 })
		for _, r := range dsResponses {
			dsDropFields(p, r, func(n int32) bool { return n <= 3 })
		}
		for _, e := range p.EnumType {
			if e.GetName() == "SortBy" {
				var vs []*descriptorpb.EnumValueDescriptorProto
				for _, v := range e.Value {
					if v.GetNumber() <= 4 {
						vs = append(vs, v)
					}
				}
				e.Value = vs
			}
		}
	})
	return fd, old
}

func dsSortByValues(t *testing.T, fd protoreflect.FileDescriptor) map[string]int32 {
	t.Helper()
	ed := fd.Enums().ByName("SortBy")
	if ed == nil {
		t.Fatalf("enum SortBy missing")
	}
	got := map[string]int32{}
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		got[string(v.Name())] = int32(v.Number())
	}
	return got
}

func TestDiscovery_SortByBestSellingIsFive(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	want := map[string]int32{"SORT_BY_BEST_SELLING": 5}
	for k, v := range dsBaselineSortBy {
		want[k] = v
	}
	got := dsSortByValues(t, fd)
	if len(got) != len(want) {
		t.Errorf("SortBy = %v, want exactly %v", got, want)
	}
	for k, v := range want {
		if g, ok := got[k]; !ok || g != v {
			t.Errorf("SortBy.%s = %d (present=%v), want %d", k, g, ok, v)
		}
	}
}

func TestDiscovery_BestSellingOrderingDocumented(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	v := fd.Enums().ByName("SortBy").Values().ByName("SORT_BY_BEST_SELLING")
	if v == nil {
		t.Fatalf("SORT_BY_BEST_SELLING missing")
	}
	dsCheck(t, "SORT_BY_BEST_SELLING", dsComment(fd, v), "sold_count desc, created_at desc, id")
}

func TestDiscovery_OldSchemaParsesBestSelling(t *testing.T) {
	fd, old := dsOldSearch(t)
	if fd.Enums().ByName("SortBy").Values().ByNumber(5) == nil {
		t.Fatalf("new schema lacks SortBy number 5")
	}
	newReq := dsMsg(t, fd, "SearchListingsRequest")
	oldReq := dsMsg(t, old, "SearchListingsRequest")
	sortBy := dsFld(t, newReq, "sort_by")
	m := dynamicpb.NewMessage(newReq)
	dsSet(t, m, "query", "shoes")
	m.Set(sortBy, protoreflect.ValueOfEnum(5))
	got := dsParse(t, m, oldReq)
	if n := got.Get(dsFld(t, oldReq, "sort_by")).Enum(); n != 5 {
		t.Errorf("old schema reads sort_by = %d, want 5", n)
	}
	if old.Enums().ByName("SortBy").Values().ByNumber(5) != nil {
		t.Errorf("old schema must not define a name for 5")
	}
	if dsStr(got, "query") != "shoes" {
		t.Errorf("query changed: %q", dsStr(got, "query"))
	}
	omitted := dsParse(t, dsNew(t, newReq, "query", "shoes"), newReq)
	if n := omitted.Get(sortBy).Enum(); n != 0 {
		t.Errorf("omitted sort_by reads %d, want SORT_BY_UNSPECIFIED", n)
	}
}

func TestDiscovery_NoUndefinedSortNumbers(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	if v := fd.Enums().ByName("SortBy").Values().ByNumber(6); v != nil {
		t.Errorf("SortBy defines number 6 (%s); adding a value needs a spec change", v.Name())
	}
}

func TestDiscovery_ResponsesCarryAttributionFields(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	for _, r := range dsResponses {
		want := map[string]string{"request_id": "4 string", "model_version": "5 string"}
		for k, v := range dsBaselineResponse {
			want[k] = v
		}
		dsShape(t, fd, r, want)
	}
}

func TestDiscovery_AttributionRoundTrip(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	for _, r := range dsResponses {
		md := dsMsg(t, fd, r)
		m := dsNew(t, md, "request_id", "a1b2", "model_version", "hybrid+rerank@v1")
		got := dsRoundEqual(t, m)
		if dsStr(got, "request_id") != "a1b2" || dsStr(got, "model_version") != "hybrid+rerank@v1" {
			t.Errorf("%s attribution lost: %v", r, got)
		}
		empty := dsParse(t, dynamicpb.NewMessage(md), md)
		if dsStr(empty, "request_id") != "" || dsStr(empty, "model_version") != "" {
			t.Errorf("%s empty response must read empty attribution: %v", r, empty)
		}
	}
}

func TestDiscovery_AttributionWireCompatible(t *testing.T) {
	fd, old := dsOldSearch(t)
	for _, r := range dsResponses {
		newMD, oldMD := dsMsg(t, fd, r), dsMsg(t, old, r)
		newHit := dsNew(t, dsMsg(t, fd, "SearchHit"), "listing_id", "l-1")
		oldHit := dsNew(t, dsMsg(t, old, "SearchHit"), "listing_id", "l-1")

		oldMsg := dynamicpb.NewMessage(oldMD)
		oldMsg.Mutable(dsFld(t, oldMD, "hits")).List().Append(protoreflect.ValueOfMessage(oldHit))
		fromOld := dsParse(t, oldMsg, newMD)
		hits := fromOld.Get(dsFld(t, newMD, "hits")).List()
		if hits.Len() != 1 || dsStr(hits.Get(0).Message(), "listing_id") != "l-1" {
			t.Errorf("%s: old payload hits changed: %v", r, fromOld)
		}
		if dsStr(fromOld, "request_id") != "" || dsStr(fromOld, "model_version") != "" {
			t.Errorf("%s: old payload must read empty attribution", r)
		}

		newMsg := dsNew(t, newMD, "request_id", "a1b2", "model_version", "lexical@v1")
		newMsg.Mutable(dsFld(t, newMD, "hits")).List().Append(protoreflect.ValueOfMessage(newHit))
		toOld := dsParse(t, newMsg, oldMD)
		oh := toOld.Get(dsFld(t, oldMD, "hits")).List()
		if oh.Len() != 1 || dsStr(oh.Get(0).Message(), "listing_id") != "l-1" {
			t.Errorf("%s: new payload hits changed for old reader: %v", r, toOld)
		}
	}
}

func TestDiscovery_ModelVersionFormatDocumented(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	for _, r := range dsResponses {
		dsCheck(t, r+".model_version", dsFieldComment(t, fd, r, "model_version"),
			"<mode>[+rerank]@<version>", "mode that actually ran")
	}
}

func TestDiscovery_SearchHitFieldsStable(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	dsShape(t, fd, "SearchHit", map[string]string{
		"listing_id": "1 string", "score": "2 float", "stock": "3 optional int32",
		"sold_count": "4 int64", "rating_avg": "5 double", "review_count": "6 int64", "seller_id": "7 string",
	})
}

func TestDiscovery_ExistingHitFieldsNotRenumbered(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	dsKeepsBaseline(t, fd, "SearchHit", dsBaselineHit)
}

func TestDiscovery_OldHitReadsZeroes(t *testing.T) {
	fd, old := dsOldSearch(t)
	newMD, oldMD := dsMsg(t, fd, "SearchHit"), dsMsg(t, old, "SearchHit")
	for _, f := range []string{"sold_count", "rating_avg", "review_count", "seller_id"} {
		dsFld(t, newMD, f)
	}
	oldHit := dsNew(t, oldMD, "listing_id", "l-1", "score", float32(0.5), "stock", int32(7))
	got := dsParse(t, oldHit, newMD)
	if dsStr(got, "listing_id") != "l-1" || dsFloat(got, "score") != 0.5 || dsInt(got, "stock") != 7 {
		t.Errorf("existing hit fields changed: %v", got)
	}
	if !got.Has(dsFld(t, newMD, "stock")) {
		t.Errorf("stock presence lost")
	}
	if dsInt(got, "sold_count") != 0 || dsFloat(got, "rating_avg") != 0 || dsInt(got, "review_count") != 0 || dsStr(got, "seller_id") != "" {
		t.Errorf("unprojected hit must read zeroes and empty seller_id: %v", got)
	}
}

func TestDiscovery_HitBoundaryValuesRoundTrip(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	m := dsNew(t, dsMsg(t, fd, "SearchHit"), "listing_id", "l-1",
		"sold_count", int64(9223372036854775807), "rating_avg", float64(5),
		"review_count", int64(9223372036854775807), "seller_id", "s-1")
	got := dsRoundEqual(t, m)
	if dsInt(got, "sold_count") != 9223372036854775807 || dsInt(got, "review_count") != 9223372036854775807 {
		t.Errorf("maximum counts lost: %v", got)
	}
}

func TestDiscovery_RatingSemanticsDocumented(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	var all []string
	for _, f := range []string{"sold_count", "rating_avg", "review_count", "seller_id"} {
		all = append(all, dsFieldComment(t, fd, "SearchHit", f))
	}
	dsCheck(t, "SearchHit rating semantics", strings.Join(all, " "),
		"verified, non-hidden reviews", "0 when review_count is 0", "within 0 to 5", "never below 0", "empty when not yet projected")
	dsCheck(t, "SearchHit.rating_avg", all[1], "verified, non-hidden reviews", "0 when review_count is 0", "within 0 to 5")
	dsCheck(t, "SearchHit.sold_count", all[0], "never below 0")
	dsCheck(t, "SearchHit.review_count", all[2], "never below 0")
	dsCheck(t, "SearchHit.seller_id", all[3], "empty when not yet projected")
}

func TestDiscovery_SellerIdRoundTrip(t *testing.T) {
	fd, old := dsOldSearch(t)
	newMD := dsMsg(t, fd, "SearchHit")
	got := dsRoundEqual(t, dsNew(t, newMD, "listing_id", "l-1", "seller_id", "s-1"))
	if dsStr(got, "seller_id") != "s-1" {
		t.Errorf("seller_id lost: %v", got)
	}
	oldHit := dsNew(t, dsMsg(t, old, "SearchHit"), "listing_id", "l-1")
	if s := dsStr(dsParse(t, oldHit, newMD), "seller_id"); s != "" {
		t.Errorf("old hit must read empty seller_id, got %q", s)
	}
}

func TestDiscovery_HitHasNoPrivateFields(t *testing.T) {
	fd, _ := dsFile(t, dsSearchProto)
	dsNoFields(t, dsMsg(t, fd, "SearchHit"), "user_id", "buyer_id", "email", "phone", "rating_by_user")
}

const dsTS = "message:google.protobuf.Timestamp"

func dsFile(t *testing.T, path string) (protoreflect.FileDescriptor, *protoregistry.Files) {
	t.Helper()
	cmd := exec.Command("buf", "build", "-o", "-#format=binpb")
	cmd.Dir = "../../.."
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

func dsMsg(t *testing.T, fd protoreflect.FileDescriptor, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(name))
	if md == nil {
		t.Fatalf("message %s missing from %s", name, fd.Path())
	}
	return md
}

func dsFld(t *testing.T, md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	t.Helper()
	f := md.Fields().ByName(protoreflect.Name(name))
	if f == nil {
		t.Fatalf("%s.%s missing", md.Name(), name)
	}
	return f
}

func dsKind(f protoreflect.FieldDescriptor) string {
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
	if f.HasOptionalKeyword() {
		s = "optional " + s
	}
	return s
}

func dsFields(md protoreflect.MessageDescriptor) map[string]string {
	got := map[string]string{}
	for i := 0; i < md.Fields().Len(); i++ {
		f := md.Fields().Get(i)
		got[string(f.Name())] = fmt.Sprintf("%d %s", f.Number(), dsKind(f))
	}
	return got
}

func dsShape(t *testing.T, fd protoreflect.FileDescriptor, msgName string, want map[string]string) {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(msgName))
	if md == nil {
		t.Errorf("message %s missing", msgName)
		return
	}
	got := dsFields(md)
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s.%s missing (want %s)", msgName, name, w)
		} else if g != w {
			t.Errorf("%s.%s = %q, want %q", msgName, name, g, w)
		}
	}
	for name, g := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s has unexpected field %s = %q", msgName, name, g)
		}
	}
}

func dsKeepsBaseline(t *testing.T, fd protoreflect.FileDescriptor, msgName string, baseline map[string]string) {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(msgName))
	if md == nil {
		t.Errorf("message %s missing", msgName)
		return
	}
	got := dsFields(md)
	for name, w := range baseline {
		if g, ok := got[name]; !ok {
			t.Errorf("%s.%s removed or renamed (baseline %s)", msgName, name, w)
		} else if g != w {
			t.Errorf("%s.%s = %q, baseline %q", msgName, name, g, w)
		}
	}
}

func dsNoFields(t *testing.T, md protoreflect.MessageDescriptor, forbidden ...string) {
	t.Helper()
	for _, name := range forbidden {
		if md.Fields().ByName(protoreflect.Name(name)) != nil {
			t.Errorf("%s must not carry %s", md.Name(), name)
		}
	}
}

func dsNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

func dsComment(fd protoreflect.FileDescriptor, d protoreflect.Descriptor) string {
	loc := fd.SourceLocations().ByDescriptor(d)
	return dsNorm(loc.LeadingComments + " " + loc.TrailingComments)
}

func dsFieldComment(t *testing.T, fd protoreflect.FileDescriptor, msg, field string) string {
	t.Helper()
	return dsComment(fd, dsFld(t, dsMsg(t, fd, msg), field))
}

func dsCheck(t *testing.T, what, comment string, keys ...string) {
	t.Helper()
	c := dsNorm(comment)
	if c == "" {
		t.Errorf("%s: no comment found on element", what)
		return
	}
	var missing []string
	for _, k := range keys {
		if !strings.Contains(c, dsNorm(k)) {
			missing = append(missing, fmt.Sprintf("%q", k))
		}
	}
	if len(missing) > 0 {
		t.Errorf("%s: comment missing %s; got:\n%s", what, strings.Join(missing, ", "), c)
	}
}

func dsOld(t *testing.T, files *protoregistry.Files, fd protoreflect.FileDescriptor, mutate func(*descriptorpb.FileDescriptorProto)) protoreflect.FileDescriptor {
	t.Helper()
	p := protodesc.ToFileDescriptorProto(fd)
	mutate(p)
	old, err := protodesc.NewFile(p, files)
	if err != nil {
		t.Fatalf("build old-schema file: %v", err)
	}
	return old
}

func dsDropFields(p *descriptorpb.FileDescriptorProto, msg string, keep func(num int32) bool) {
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

func dsSet(t *testing.T, m *dynamicpb.Message, name string, v any) {
	t.Helper()
	f := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if f == nil {
		t.Fatalf("no field %s on %s", name, m.Descriptor().Name())
	}
	switch x := v.(type) {
	case string:
		m.Set(f, protoreflect.ValueOfString(x))
	case int64:
		m.Set(f, protoreflect.ValueOfInt64(x))
	case int32:
		m.Set(f, protoreflect.ValueOfInt32(x))
	case float64:
		m.Set(f, protoreflect.ValueOfFloat64(x))
	case float32:
		m.Set(f, protoreflect.ValueOfFloat32(x))
	case uint32:
		m.Set(f, protoreflect.ValueOfUint32(x))
	case []string:
		l := m.Mutable(f).List()
		for _, s := range x {
			l.Append(protoreflect.ValueOfString(s))
		}
	case *dynamicpb.Message:
		m.Set(f, protoreflect.ValueOfMessage(x))
	default:
		t.Fatalf("unsupported value type %T", v)
	}
}

func dsNew(t *testing.T, md protoreflect.MessageDescriptor, kv ...any) *dynamicpb.Message {
	t.Helper()
	m := dynamicpb.NewMessage(md)
	for i := 0; i < len(kv); i += 2 {
		dsSet(t, m, kv[i].(string), kv[i+1])
	}
	return m
}

func dsTimestamp(f protoreflect.FieldDescriptor, seconds int64) *dynamicpb.Message {
	ts := dynamicpb.NewMessage(f.Message())
	ts.Set(f.Message().Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	return ts
}

func dsParse(t *testing.T, m proto.Message, into protoreflect.MessageDescriptor) *dynamicpb.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(into)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("parse as %s: %v", into.FullName(), err)
	}
	return got
}

func dsRoundEqual(t *testing.T, m *dynamicpb.Message) *dynamicpb.Message {
	t.Helper()
	got := dsParse(t, m, m.Descriptor())
	if !proto.Equal(got, m) {
		t.Errorf("%s changed in round trip:\n got %v\nwant %v", m.Descriptor().Name(), got, m)
	}
	return got
}

func dsStr(m protoreflect.Message, name string) string {
	return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name))).String()
}

func dsInt(m protoreflect.Message, name string) int64 {
	return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name))).Int()
}

func dsFloat(m protoreflect.Message, name string) float64 {
	return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name))).Float()
}
