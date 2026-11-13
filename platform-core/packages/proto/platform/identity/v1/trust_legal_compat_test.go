package identityv1_test

// Contract tests for change trust-legal-contract: identity half (RegisterRequest versions,
// USER_STATUS_DELETED, PrivacyService, UserErased). Reuses the tc* helpers of userstatus_compat_test.go.

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const tlPriv = "platform.identity.v1."

func tlConsentList() string { return "repeated message:" + tlPriv + "ConsentRecord" }

// Scenario: Trust legal contract: RegisterRequest carries both legal versions
func TestTrustLegal_RegisterRequestCarriesBothLegalVersions(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcShape(t, fd, "RegisterRequest", map[string]string{
		"username": "1 string", "password": "2 string", "role": "3 string", "referral_code": "4 string",
		"terms_version": "5 string", "privacy_version": "6 string",
	})
}

// Scenario: Trust legal contract: UserStatus gains DELETED without moving values
func TestTrustLegal_UserStatusGainsDeletedWithoutMovingValues(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcEnum(t, fd, "UserStatus", map[string]int{
		"USER_STATUS_UNSPECIFIED": 0, "USER_STATUS_ACTIVE": 1, "USER_STATUS_RESTRICTED": 2,
		"USER_STATUS_SUSPENDED": 3, "USER_STATUS_BANNED": 4, "USER_STATUS_DELETED": 5,
	})
}

// Scenario: Trust legal contract: PrivacyService has six RPCs and fixed messages
func TestTrustLegal_PrivacyServiceHasSixRPCsAndFixedMessages(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcRPCs(t, fd, "PrivacyService", map[string][2]string{
		"RecordConsent":            {"RecordConsentRequest", "RecordConsentResponse"},
		"ListMyConsents":           {"ListMyConsentsRequest", "ListMyConsentsResponse"},
		"RequestAccountDeletion":   {"RequestAccountDeletionRequest", "RequestAccountDeletionResponse"},
		"CancelAccountDeletion":    {"CancelAccountDeletionRequest", "CancelAccountDeletionResponse"},
		"GetAccountDeletionStatus": {"GetAccountDeletionStatusRequest", "GetAccountDeletionStatusResponse"},
		"ExportMyData":             {"ExportMyDataRequest", "ExportMyDataResponse"},
	})
	tcShape(t, fd, "ConsentRecord", map[string]string{
		"purpose": "1 enum:" + tlPriv + "ConsentPurpose", "granted": "2 bool", "version": "3 string",
		"recorded_at": "4 " + idTS, "source": "5 string",
	})
	tcShape(t, fd, "RecordConsentRequest", map[string]string{"records": "1 " + tlConsentList()})
	tcShape(t, fd, "RecordConsentResponse", map[string]string{})
	tcShape(t, fd, "ListMyConsentsRequest", map[string]string{"page": "1 " + idPageReq})
	tcShape(t, fd, "ListMyConsentsResponse", map[string]string{"records": "1 " + tlConsentList(), "page": "2 " + idPageResp})
	tcShape(t, fd, "RequestAccountDeletionRequest", map[string]string{"password": "1 string"})
	tcShape(t, fd, "RequestAccountDeletionResponse", map[string]string{"erase_after": "1 " + idTS})
	tcShape(t, fd, "CancelAccountDeletionRequest", map[string]string{})
	tcShape(t, fd, "CancelAccountDeletionResponse", map[string]string{})
	tcShape(t, fd, "GetAccountDeletionStatusRequest", map[string]string{})
	tcShape(t, fd, "GetAccountDeletionStatusResponse", map[string]string{"pending": "1 bool", "erase_after": "2 " + idTS})
	tcShape(t, fd, "ExportMyDataRequest", map[string]string{})
	tcShape(t, fd, "ExportMyDataResponse", map[string]string{"data": "1 bytes", "content_type": "2 string", "generated_at": "3 " + idTS})
}

// Scenario: Trust legal contract: ConsentPurpose has fixed names and numbers
func TestTrustLegal_ConsentPurposeHasFixedNamesAndNumbers(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcEnum(t, fd, "ConsentPurpose", map[string]int{
		"CONSENT_PURPOSE_UNSPECIFIED": 0, "CONSENT_PURPOSE_ANALYTICS": 1, "CONSENT_PURPOSE_MARKETING": 2,
		"CONSENT_PURPOSE_PERSONALIZATION": 3, "CONSENT_PURPOSE_TERMS": 4, "CONSENT_PURPOSE_PRIVACY_POLICY": 5,
		"CONSENT_PURPOSE_SELLER_TERMS": 6,
	})
}

// Scenario: Trust legal contract: ConsentRecord round-trips with a 32-character version
func TestTrustLegal_ConsentRecordRoundTripsWith32CharVersion(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	cr := tcMsg(t, fd, "ConsentRecord")
	ver := strings.Repeat("v", 32)
	m := tcNew(t, cr, "purpose", 4, "version", ver, "source", "signup",
		"recorded_at", tcTS(tcFld(t, cr, "recorded_at"), 1760000000))
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, cr, "granted")).Bool() {
		t.Errorf("granted false became true")
	}
	if got.Get(tcFld(t, cr, "version")).String() != ver {
		t.Errorf("version changed")
	}
	unset := tcNew(t, cr, "purpose", 1, "version", ver)
	if tcRoundEqual(t, unset).Has(tcFld(t, cr, "recorded_at")) {
		t.Errorf("unset recorded_at became set")
	}
}

// Scenario: Trust legal contract: ExportMyData response round-trips a 1 MiB payload
func TestTrustLegal_ExportMyDataResponseRoundTrips1MiB(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	md := tcMsg(t, fd, "ExportMyDataResponse")
	data := make([]byte, 1048576)
	for i := range data {
		data[i] = byte(i % 251)
	}
	m := tcNew(t, md, "content_type", "application/json", "generated_at", tcTS(tcFld(t, md, "generated_at"), 1760000000))
	m.Set(tcFld(t, md, "data"), protoreflect.ValueOfBytes(data))
	got := tcRoundEqual(t, m)
	if !bytes.Equal(got.Get(tcFld(t, md, "data")).Bytes(), data) {
		t.Errorf("data not byte-equal after round trip")
	}
	if got.Get(tcFld(t, md, "content_type")).String() != "application/json" {
		t.Errorf("content_type changed")
	}
}

// Scenario: Trust legal contract: Old readers skip the register versions
func TestTrustLegal_OldReadersSkipRegisterVersions(t *testing.T) {
	fd, files := tcFile(t, idProto)
	m := tcNew(t, tcMsg(t, fd, "RegisterRequest"), "username", "bob", "password", "pw", "role", "buyer",
		"referral_code", "ABC", "terms_version", "2026-11", "privacy_version", "2026-11")
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropFields(p, "RegisterRequest", func(n int32) bool { return n <= 4 })
	})
	om := old.Messages().ByName("RegisterRequest")
	got := tcParseWith(t, m, om)
	for f, want := range map[string]string{"username": "bob", "password": "pw", "role": "buyer", "referral_code": "ABC"} {
		if got.Get(om.Fields().ByName(protoreflect.Name(f))).String() != want {
			t.Errorf("old reader lost %s", f)
		}
	}
}

// Scenario: Trust legal contract: Old readers parse an unknown status as a number
func TestTrustLegal_OldReadersParseUnknownStatusAsNumber(t *testing.T) {
	fd, files := tcFile(t, idProto)
	if fd.Enums().ByName("UserStatus").Values().ByNumber(5) == nil {
		t.Fatalf("UserStatus has no value 5")
	}
	m := tcNew(t, tcMsg(t, fd, "AdminUser"), "id", "u-1", "username", "bob", "status", 5)
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropEnumValues(p, "UserStatus", func(n int32) bool { return n <= 4 })
	})
	om := old.Messages().ByName("AdminUser")
	got := tcParseWith(t, m, om)
	if got.Get(om.Fields().ByName("status")).Enum() != 5 {
		t.Errorf("unknown status 5 not kept as a number")
	}
}

// Scenario: Trust legal contract: Privacy principal rules are documented
func TestTrustLegal_PrivacyPrincipalRulesAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcCheck(t, "PrivacyService", tcComment(fd, fd.Services().ByName("PrivacyService")),
		"USER principal", "UNAUTHENTICATED", "PERMISSION_DENIED", "no request field")
	tcCheck(t, "RequestAccountDeletion", tcRPCComment(t, fd, "PrivacyService", "RequestAccountDeletion"),
		"password", "PERMISSION_DENIED", "FAILED_PRECONDITION")
	tcCheck(t, "ExportMyData", tcRPCComment(t, fd, "PrivacyService", "ExportMyData"), "1 MiB")
}

// Scenario: Trust legal contract: Register and DELETED semantics are documented
func TestTrustLegal_RegisterAndDeletedSemanticsAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcCheck(t, "terms_version", tcFieldComment(t, fd, "RegisterRequest", "terms_version"),
		"INVALID_ARGUMENT", "current configured version")
	tcCheck(t, "privacy_version", tcFieldComment(t, fd, "RegisterRequest", "privacy_version"),
		"INVALID_ARGUMENT", "current configured version")
	v := fd.Enums().ByName("UserStatus").Values().ByName("USER_STATUS_DELETED")
	if v == nil {
		t.Fatalf("USER_STATUS_DELETED missing")
	}
	tcCheck(t, "USER_STATUS_DELETED", tcComment(fd, v), "erasure sweeper", "unknown user")
}

// Scenario: Trust legal contract: Missing principal note is caught
func TestTrustLegal_MissingPrincipalNoteIsCaught(t *testing.T) {
	fixture := "PrivacyService accepts anonymous UNAUTHENTICATED and PERMISSION_DENIED, no request field selects a user."
	if err := tcKeys(fixture, "USER principal", "UNAUTHENTICATED", "PERMISSION_DENIED", "no request field"); err == nil {
		t.Errorf("helper did not report the missing USER principal note")
	}
}

// Scenario: Trust legal contract: UserErased message is fixed
func TestTrustLegal_UserErasedMessageIsFixed(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcShape(t, fd, "UserErased", map[string]string{"user_id": "1 string", "roles": "2 repeated string", "erased_at": "3 " + idTS})
}

// Scenario: Trust legal contract: UserErased round-trips with several roles
func TestTrustLegal_UserErasedRoundTripsWithSeveralRoles(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	md := tcMsg(t, fd, "UserErased")
	id := "01HZX3K9M2N4P5Q6R7S8T9V0WA"
	m := tcNew(t, md, "user_id", id, "roles", []string{"buyer", "seller"}, "erased_at", tcTS(tcFld(t, md, "erased_at"), 1760000000))
	got := tcRoundEqual(t, m)
	l := got.Get(tcFld(t, md, "roles")).List()
	if got.Get(tcFld(t, md, "user_id")).String() != id || l.Len() != 2 || l.Get(0).String() != "buyer" || l.Get(1).String() != "seller" {
		t.Errorf("UserErased fields or role order changed")
	}
}

// Scenario: Trust legal contract: UserErased without roles round-trips
func TestTrustLegal_UserErasedWithoutRolesRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	md := tcMsg(t, fd, "UserErased")
	m := tcNew(t, md, "user_id", "01HZX3K9M2N4P5Q6R7S8T9V0WA")
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, md, "roles")).List().Len() != 0 || got.Get(tcFld(t, md, "user_id")).String() != "01HZX3K9M2N4P5Q6R7S8T9V0WA" {
		t.Errorf("roles not empty or user_id lost")
	}
}

// Scenario: Trust legal contract: UserErased semantics are documented
func TestTrustLegal_UserErasedSemanticsAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcCheck(t, "UserErased", tcComment(fd, tcMsg(t, fd, "UserErased")),
		"identity.events", "platform.identity.v1.UserErased", "outbox", "same transaction", "idempotent", "internal-only")
}

// Scenario: Trust legal contract: UserErased is documented as not exposed to any principal at the edge
func TestTrustLegal_UserErasedIsNotExposedAtTheEdge(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	tcCheck(t, "UserErased", tcComment(fd, tcMsg(t, fd, "UserErased")), "never exposed at the gateway")
}

// Scenario: Trust legal contract: Missing idempotency note is caught
func TestTrustLegal_MissingIdempotencyNoteIsCaught(t *testing.T) {
	fixture := "published on identity.events, platform.identity.v1.UserErased, outbox, same transaction, internal-only."
	if err := tcKeys(fixture, "identity.events", "outbox", "same transaction", "idempotent", "internal-only"); err == nil {
		t.Errorf("helper did not report the missing idempotent note")
	}
}
