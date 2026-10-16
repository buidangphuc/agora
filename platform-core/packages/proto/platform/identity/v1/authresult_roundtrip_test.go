package identityv1_test

// Contract tests for the AuthResult refresh fields and the RefreshSession RPC
// (openspec change q1-session-refresh-contract, spec identity-auth-contract).
//
// The tests compile the proto with `buf build` (source info included) and
// inspect the descriptors dynamically, so they do not depend on the
// gitignored generated code.

import (
	"bytes"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const identityProto = "platform/identity/v1/identity.proto"

func loadFile(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	cmd := exec.Command("buf", "build", "-o", "-#format=binpb")
	cmd.Dir = "../../.." // packages/proto (buf module root)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed: %v\n%s", err, stderr.String())
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(out.Bytes(), &set); err != nil {
		t.Fatalf("decode image: %v", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		t.Fatalf("build files: %v", err)
	}
	fd, err := files.FindFileByPath(identityProto)
	if err != nil {
		t.Fatalf("find %s: %v", identityProto, err)
	}
	return fd
}

func authResult(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()
	md := loadFile(t).Messages().ByName("AuthResult")
	if md == nil {
		t.Fatal("AuthResult message missing")
	}
	return md
}

func TestAuthResult_FieldsExistWithAgreedNumbers_Scenario_FieldsExist(t *testing.T) {
	md := authResult(t)
	type want struct {
		num  protoreflect.FieldNumber
		kind protoreflect.Kind
		full protoreflect.FullName
	}
	cases := map[protoreflect.Name]want{
		"token":              {1, protoreflect.StringKind, ""},
		"principal":          {2, protoreflect.MessageKind, "platform.common.v1.Principal"},
		"username":           {3, protoreflect.StringKind, ""},
		"refresh_token":      {4, protoreflect.StringKind, ""},
		"refresh_expires_at": {5, protoreflect.MessageKind, "google.protobuf.Timestamp"},
	}
	for name, w := range cases {
		f := md.Fields().ByName(name)
		if f == nil {
			t.Errorf("field %q missing", name)
			continue
		}
		if f.Number() != w.num || f.Kind() != w.kind {
			t.Errorf("field %q = number %d kind %v, want %d %v", name, f.Number(), f.Kind(), w.num, w.kind)
		}
		if w.full != "" && f.Message().FullName() != w.full {
			t.Errorf("field %q type = %s, want %s", name, f.Message().FullName(), w.full)
		}
	}
	if md.Fields().Len() != len(cases) {
		t.Errorf("AuthResult has %d fields, want exactly %d", md.Fields().Len(), len(cases))
	}
}

func TestAuthResult_RoundTripRefreshUnset_Scenario_BoundarySessionsDisabled(t *testing.T) {
	md := authResult(t)
	m := dynamicpb.NewMessage(md)
	m.Set(md.Fields().ByName("token"), protoreflect.ValueOfString("access"))
	m.Set(md.Fields().ByName("username"), protoreflect.ValueOfString("alice"))

	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatal(err)
	}
	rt := md.Fields().ByName("refresh_token")
	re := md.Fields().ByName("refresh_expires_at")
	if rt == nil || re == nil {
		t.Fatal("refresh_token / refresh_expires_at missing")
	}
	if s := got.Get(rt).String(); s != "" {
		t.Errorf("refresh_token = %q, want empty", s)
	}
	if got.Has(re) {
		t.Error("refresh_expires_at must be unset after round trip")
	}
	if got.Get(md.Fields().ByName("token")).String() != "access" ||
		got.Get(md.Fields().ByName("username")).String() != "alice" {
		t.Error("fields 1-3 not preserved")
	}
}

func TestRefreshSession_MessagesExist_Scenario_ServiceAndMessagesExist(t *testing.T) {
	fd := loadFile(t)
	req := fd.Messages().ByName("RefreshSessionRequest")
	if req == nil {
		t.Fatal("RefreshSessionRequest missing")
	}
	f := req.Fields().ByName("refresh_token")
	if f == nil || f.Number() != 1 || f.Kind() != protoreflect.StringKind {
		t.Errorf("RefreshSessionRequest.refresh_token must be string = 1, got %v", f)
	}
	resp := fd.Messages().ByName("RefreshSessionResponse")
	if resp == nil {
		t.Fatal("RefreshSessionResponse missing")
	}
	r := resp.Fields().ByName("result")
	if r == nil || r.Number() != 1 || r.Kind() != protoreflect.MessageKind ||
		r.Message().Name() != "AuthResult" {
		t.Errorf("RefreshSessionResponse.result must be AuthResult = 1, got %v", r)
	}
}

func refreshMethod(t *testing.T) protoreflect.MethodDescriptor {
	t.Helper()
	svc := loadFile(t).Services().ByName("AuthService")
	if svc == nil {
		t.Fatal("AuthService missing")
	}
	m := svc.Methods().ByName("RefreshSession")
	if m == nil {
		t.Fatal("AuthService.RefreshSession missing")
	}
	return m
}

func TestRefreshSession_RPCSignature_Scenario_ServiceAndMessagesExist(t *testing.T) {
	m := refreshMethod(t)
	if m.Input().Name() != "RefreshSessionRequest" || m.Output().Name() != "RefreshSessionResponse" {
		t.Errorf("signature = %s -> %s", m.Input().Name(), m.Output().Name())
	}
	if m.IsStreamingClient() || m.IsStreamingServer() {
		t.Error("RefreshSession must be unary")
	}
}

func TestRefreshSession_DocComment_Scenarios_ErrorContractAndGrace(t *testing.T) {
	m := refreshMethod(t)
	fd := loadFile(t)
	loc := fd.SourceLocations().ByDescriptor(fd.Services().ByName("AuthService").Methods().ByName("RefreshSession"))
	_ = m
	c := strings.ToLower(strings.Join(strings.Fields(loc.LeadingComments), " "))
	if c == "" {
		t.Fatal("RefreshSession has no leading doc comment")
	}
	for _, w := range []string{
		"unauthenticated", "unknown", "expired", "revoked", "reused", "empty", "malformed",
		"public", "not retried", "revoke", "same sid", "refresh_token", "grace",
	} {
		if !strings.Contains(c, w) {
			t.Errorf("doc comment missing %q", w)
		}
	}
	if !regexp.MustCompile(`10 ?s`).MatchString(c) {
		t.Error("doc comment must state the 10 s grace window")
	}
	if !strings.Contains(c, "inclusive") {
		t.Error("doc comment must state the 10 s bound is inclusive")
	}
}
