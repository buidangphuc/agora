package identityv1_test

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	gcApp    = "enum:platform.identity.v1.HandoffApp"
	gcResult = "message:platform.identity.v1.AuthResult"
)

// Scenario: Hand-off RPCs and messages exist
func TestGapsContract_HandoffRPCs(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	sd := fd.Services().ByName("AuthService")
	if sd == nil {
		t.Fatalf("AuthService missing")
	}
	for rpc, io := range map[string][2]string{
		"CreateHandoffCode": {"CreateHandoffCodeRequest", "CreateHandoffCodeResponse"},
		"RedeemHandoffCode": {"RedeemHandoffCodeRequest", "RedeemHandoffCodeResponse"},
		"PeekHandoffCode":   {"PeekHandoffCodeRequest", "PeekHandoffCodeResponse"},
	} {
		m := sd.Methods().ByName(protoreflect.Name(rpc))
		if m == nil {
			t.Errorf("AuthService.%s missing", rpc)
			continue
		}
		if string(m.Input().Name()) != io[0] || string(m.Output().Name()) != io[1] {
			t.Errorf("AuthService.%s = (%s) returns (%s), want (%s) returns (%s)", rpc, m.Input().Name(), m.Output().Name(), io[0], io[1])
		}
	}
	tcShape(t, fd, "CreateHandoffCodeRequest", map[string]string{"target_app": "1 " + gcApp})
	tcShape(t, fd, "CreateHandoffCodeResponse", map[string]string{"code": "1 string", "expires_at": "2 " + idTS})
	tcShape(t, fd, "RedeemHandoffCodeRequest", map[string]string{"code": "1 string", "target_app": "2 " + gcApp})
	tcShape(t, fd, "RedeemHandoffCodeResponse", map[string]string{"result": "1 " + gcResult})
	tcShape(t, fd, "PeekHandoffCodeRequest", map[string]string{"code": "1 string", "target_app": "2 " + gcApp})
	tcShape(t, fd, "PeekHandoffCodeResponse", map[string]string{"username": "1 string", "expires_at": "2 " + idTS})
}

// Scenario: BecomeSeller and event exist
func TestGapsContract_BecomeSellerAndEvent(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	sd := fd.Services().ByName("AuthService")
	if sd == nil {
		t.Fatalf("AuthService missing")
	}
	m := sd.Methods().ByName("BecomeSeller")
	if m == nil {
		t.Errorf("AuthService.BecomeSeller missing")
	} else if string(m.Input().Name()) != "BecomeSellerRequest" || string(m.Output().Name()) != "BecomeSellerResponse" {
		t.Errorf("AuthService.BecomeSeller = (%s) returns (%s)", m.Input().Name(), m.Output().Name())
	}
	tcShape(t, fd, "BecomeSellerRequest", map[string]string{"terms_version": "1 string"})
	tcShape(t, fd, "BecomeSellerResponse", map[string]string{"result": "1 " + gcResult})
	tcEnum(t, fd, "HandoffApp", map[string]int{
		"HANDOFF_APP_UNSPECIFIED": 0, "HANDOFF_APP_BUYER": 1, "HANDOFF_APP_SELLER": 2,
	})
	tcShape(t, fd, "UserRoleGranted", map[string]string{
		"user_id": "1 string", "role": "2 string", "granted_at": "3 " + idTS,
	})
}

// Scenario: Unspecified target is the zero value
func TestGapsContract_UnspecifiedTargetIsZero(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	md := tcMsg(t, fd, "CreateHandoffCodeRequest")
	f := tcFld(t, md, "target_app")
	m := tcNew(t, md)
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(b) != 0 {
		t.Errorf("empty CreateHandoffCodeRequest wire size = %d, want 0", len(b))
	}
	got := tcRound(t, m)
	if n := got.Get(f).Enum(); n != 0 {
		t.Errorf("target_app decoded as %d, want 0 (HANDOFF_APP_UNSPECIFIED)", n)
	}
	if f.Enum().Values().ByNumber(0) == nil || f.Enum().Values().ByNumber(0).Name() != "HANDOFF_APP_UNSPECIFIED" {
		t.Errorf("enum value 0 of HandoffApp is not HANDOFF_APP_UNSPECIFIED")
	}
}
