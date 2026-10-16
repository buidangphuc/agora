package identityv1_test

// Contract tests for change q6-trust-contract (capability trust-contract): AccountAdminService, UserStatusChanged.

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

const idProto = "platform/identity/v1/identity.proto"
const idPageReq = "message:platform.common.v1.PageRequest"
const idPageResp = "message:platform.common.v1.PageResponse"
const idTS = "message:google.protobuf.Timestamp"
const idStatus = "enum:platform.identity.v1.UserStatus"

// Scenario: Trust contract: AccountAdminService exists
func TestTrustContract_AccountAdminServiceExists(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcRPCs(t, fd, "AccountAdminService", map[string][2]string{
		"ListUsers":     {"ListUsersRequest", "ListUsersResponse"},
		"SetUserStatus": {"SetUserStatusRequest", "SetUserStatusResponse"},
	})
	for _, sr := range [][2]string{{"AuthService", "Login"}, {"AuthService", "RefreshSession"},
		{"AddressService", "ListAddresses"}, {"SessionService", "RevokeSession"}, {"PublicProfileService", "GetPublicProfiles"}} {
		svc, rpc := sr[0], sr[1]
		if fd.Services().ByName(protoreflect.Name(svc)) == nil ||
			fd.Services().ByName(protoreflect.Name(svc)).Methods().ByName(protoreflect.Name(rpc)) == nil {
			t.Errorf("existing %s.%s missing", svc, rpc)
		}
	}
}

// Scenario: Trust contract: User status messages have fixed numbers
func TestTrustContract_UserStatusMessagesHaveFixedNumbers(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcEnum(t, fd, "UserStatus", map[string]int{
		"USER_STATUS_UNSPECIFIED": 0, "USER_STATUS_ACTIVE": 1, "USER_STATUS_RESTRICTED": 2,
		"USER_STATUS_SUSPENDED": 3, "USER_STATUS_BANNED": 4,
	})
	tcShape(t, fd, "AdminUser", map[string]string{
		"id": "1 string", "username": "2 string", "roles": "3 repeated string", "status": "4 " + idStatus,
		"status_reason": "5 string", "status_until": "6 " + idTS, "created_at": "7 " + idTS,
	})
	tcShape(t, fd, "ListUsersRequest", map[string]string{"query": "1 string", "status": "2 " + idStatus, "page": "3 " + idPageReq})
	tcShape(t, fd, "ListUsersResponse", map[string]string{
		"users": "1 repeated message:platform.identity.v1.AdminUser", "page": "2 " + idPageResp,
	})
	tcShape(t, fd, "SetUserStatusRequest", map[string]string{
		"user_id": "1 string", "status": "2 " + idStatus, "reason_code": "3 string", "note": "4 string", "until": "5 " + idTS,
	})
	tcShape(t, fd, "SetUserStatusResponse", map[string]string{
		"user": "1 message:platform.identity.v1.AdminUser", "sessions_revoked": "2 int32|2 int64|2 uint32|2 uint64",
	})
	tcShape(t, fd, "UserStatusChanged", map[string]string{
		"user_id": "1 string", "status": "2 " + idStatus, "previous_status": "3 " + idStatus,
		"reason_code": "4 string", "until": "5 " + idTS, "changed_at": "6 " + idTS,
	})
}

// Scenario: Trust contract: UserStatusChanged round-trips
func TestTrustContract_UserStatusChangedRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	ev := tcMsg(t, fd, "UserStatusChanged")
	m := tcNew(t, ev, "user_id", "u-1", "status", 3, "previous_status", 1, "reason_code", "counterfeit_seller",
		"until", tcTS(tcFld(t, ev, "until"), 1760100000), "changed_at", tcTS(tcFld(t, ev, "changed_at"), 1760000000))
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, ev, "status")).Enum() != 3 || got.Get(tcFld(t, ev, "previous_status")).Enum() != 1 {
		t.Errorf("status fields changed in round trip")
	}
	req := tcMsg(t, fd, "SetUserStatusRequest")
	r := tcNew(t, req, "user_id", "u-1", "status", 4, "reason_code", "scam")
	gotReq := tcRoundEqual(t, r)
	if gotReq.Has(tcFld(t, req, "until")) {
		t.Errorf("unset until became set after round trip")
	}
}

// Scenario: Trust contract: Old readers tolerate unknown user status
func TestTrustContract_OldReadersToleratedUnknownUserStatus(t *testing.T) {
	fd, files := tcFile(t, idProto)
	m := tcNew(t, tcMsg(t, fd, "AdminUser"), "id", "u-1", "username", "bob", "status", 4)
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropEnumValues(p, "UserStatus", func(n int32) bool { return n <= 2 })
	})
	oldMsg := old.Messages().ByName("AdminUser")
	got := tcParseWith(t, m, oldMsg)
	if got.Get(oldMsg.Fields().ByName("status")).Enum() != 4 {
		t.Errorf("unknown status 4 not kept as a number")
	}
}

// Scenario: Trust contract: SetUserStatus authz, audit and errors are documented
func TestTrustContract_SetUserStatusAuthzAuditAndErrorsAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	c := tcRPCComment(t, fd, "AccountAdminService", "SetUserStatus")
	tcCheck(t, "SetUserStatus", c, "admin-only", "scope admin", "UNAUTHENTICATED", "PERMISSION_DENIED",
		"user.set_status", "first", "UNAVAILABLE", "no change",
		"FAILED_PRECONDITION", "caller's own id", "admin target",
		"INVALID_ARGUMENT", "status UNSPECIFIED", "lower_snake_case", "64", "note over 500",
		"until missing or not in the future for SUSPENDED", "until set for any other status",
		"NOT_FOUND", "one UserStatusChanged", "RESTRICTED, SUSPENDED or BANNED",
		"revokes every session", "one SessionRevoked per session", "re-setting the current status", "ACTIVE revokes none")
}

// Scenario: Trust contract: ListUsers is documented as a read
func TestTrustContract_ListUsersIsDocumentedAsARead(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	c := tcRPCComment(t, fd, "AccountAdminService", "ListUsers")
	tcCheck(t, "ListUsers", c, "admin-only", "username prefix or an exact id", "UNSPECIFIED status means all", "not audited")
}

// Scenario: Trust contract: Event and login refusal are documented
func TestTrustContract_EventAndLoginRefusalAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcCheck(t, "UserStatusChanged", tcComment(fd, tcMsg(t, fd, "UserStatusChanged")),
		"identity.events", "key = user id", "platform.identity.v1.UserStatusChanged")
	login := tcRPCComment(t, fd, "AuthService", "Login")
	refresh := tcRPCComment(t, fd, "AuthService", "RefreshSession")
	for name, c := range map[string]string{"Login": login, "RefreshSession": refresh} {
		tcCheck(t, name, c, "SUSPENDED or BANNED", "PERMISSION_DENIED", "one generic message",
			"does not tell suspended from banned", "does not reveal an end time")
	}
	tcCheck(t, "Login", login, "expired suspension is lifted to ACTIVE at that login")
	tcCheck(t, "RefreshSession", refresh, "never lifts")
}

// Scenario: Trust contract: Non-admin callers are denied user administration
func TestTrustContract_NonAdminCallersAreDeniedUserAdministration(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	c := tcComment(fd, fd.Services().ByName("AccountAdminService"))
	tcCheck(t, "AccountAdminService", c, "without scope admin gets PERMISSION_DENIED",
		"without a principal gets UNAUTHENTICATED", "both RPCs")
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
