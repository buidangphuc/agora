package identityv1_test

// Contract tests for change trust-legal-contract: identity half (RegisterRequest versions,
// USER_STATUS_DELETED, PrivacyService, UserErased). Reuses the tc* helpers of userstatus_compat_test.go.

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
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

// ---- order, payment, verification, audit half (task T2) ----

const (
	tlOrderProto = "platform/order/v1/order.proto"
	tlPayProto   = "platform/payment/v1/payment.proto"
	tlVerProto   = "platform/verification/v1/verification.proto"
	tlAuditProto = "platform/audit/v1/audit.proto"
	tlVer        = "platform.verification.v1."
	tlMaxInt64   = 9223372036854775807
)

func tlRPCMap(names ...string) map[string][2]string {
	m := map[string][2]string{}
	for _, n := range names {
		m[n] = [2]string{n + "Request", n + "Response"}
	}
	return m
}

func tlJoin(a map[string][2]string, b map[string][2]string) map[string][2]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

// Scenario: Trust legal contract: GetUserObligations exists with fixed messages
func TestTrustLegal_GetUserObligationsExistsWithFixedMessages(t *testing.T) {
	fd, _ := tcFile(t, tlOrderProto)
	tcRPCs(t, fd, "OrderService", tlRPCMap(
		"CreateOrder", "GetOrder", "ListBuyerOrders", "ListSellerOrders", "UpdateOrderStatus", "CancelOrder",
		"ConfirmReceipt", "CalculateShippingFee", "PreviewCheckout", "GetSagaState", "ForceFailSaga",
		"CreateReturnRequest", "GetReturnRequest", "UpdateReturnStatus", "ListOrderReturns", "CreateShipment",
		"GetShipmentTracking", "HasCompletedOrder", "GetUserObligations"))
	tcShape(t, fd, "GetUserObligationsRequest", map[string]string{"user_id": "1 string"})
	tcShape(t, fd, "GetUserObligationsResponse", map[string]string{
		"open_buyer_orders": "1 int32", "open_seller_orders": "2 int32", "open_returns": "3 int32"})
}

// Scenario: Trust legal contract: GetPayoutObligations exists with fixed messages
func TestTrustLegal_GetPayoutObligationsExistsWithFixedMessages(t *testing.T) {
	fd, _ := tcFile(t, tlPayProto)
	tcRPCs(t, fd, "PaymentService", tlRPCMap(
		"CreatePayment", "GetPayment", "ProcessMockPayment", "GetSellerWallet", "RequestPayout", "ListPayoutHistory",
		"RefundPayment", "GetWalletBalance", "ListLedgerEntries", "RequestWalletPayout", "GetPayoutObligations"))
	tcShape(t, fd, "GetPayoutObligationsRequest", map[string]string{"user_id": "1 string"})
	tcShape(t, fd, "GetPayoutObligationsResponse", map[string]string{
		"available_balance": "1 int64", "held_balance": "2 int64", "pending_payouts": "3 int32"})
}

// Scenario: Trust legal contract: Obligation responses round-trip extreme values
func TestTrustLegal_ObligationResponsesRoundTripExtremeValues(t *testing.T) {
	pfd, _ := tcFile(t, tlPayProto)
	pd := tcMsg(t, pfd, "GetPayoutObligationsResponse")
	m := tcNew(t, pd, "available_balance", tlMaxInt64, "held_balance", 0, "pending_payouts", 7)
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, pd, "available_balance")).Int() != tlMaxInt64 {
		t.Errorf("available_balance changed")
	}
	ofd, _ := tcFile(t, tlOrderProto)
	od := tcMsg(t, ofd, "GetUserObligationsResponse")
	z := tcNew(t, od, "open_buyer_orders", 0, "open_seller_orders", 0, "open_returns", 0)
	b, err := proto.Marshal(z)
	if err != nil || len(b) != 0 {
		t.Errorf("all-zero response should serialise empty, got %d bytes, err %v", len(b), err)
	}
	if !proto.Equal(tcRound(t, z), dynamicpb.NewMessage(od)) {
		t.Errorf("zero response did not parse back as the empty message")
	}
}

// Scenario: Trust legal contract: Obligation RPCs are documented as internal
func TestTrustLegal_ObligationRPCsAreDocumentedAsInternal(t *testing.T) {
	ofd, _ := tcFile(t, tlOrderProto)
	tcCheck(t, "GetUserObligations", tcRPCComment(t, ofd, "OrderService", "GetUserObligations"),
		"service principal", "order.internal.read", "PERMISSION_DENIED", "NOT routed at the gateway")
	pfd, _ := tcFile(t, tlPayProto)
	tcCheck(t, "GetPayoutObligations", tcRPCComment(t, pfd, "PaymentService", "GetPayoutObligations"),
		"service principal", "payment.internal.read", "PERMISSION_DENIED", "NOT routed at the gateway")
}

// Scenario: Trust legal contract: Payout gate wording is documented
func TestTrustLegal_PayoutGateWordingIsDocumented(t *testing.T) {
	fd, _ := tcFile(t, tlPayProto)
	for _, rpc := range []string{"RequestPayout", "RequestWalletPayout"} {
		tcCheck(t, rpc, tcRPCComment(t, fd, "PaymentService", rpc),
			"seller verification required", "VERIFIED", "FAILED_PRECONDITION", "UNAVAILABLE")
	}
}

// Scenario: Trust legal contract: Payout request fields are unchanged
func TestTrustLegal_PayoutRequestFieldsAreUnchanged(t *testing.T) {
	fd, _ := tcFile(t, tlPayProto)
	tcShape(t, fd, "RequestPayoutRequest", map[string]string{
		"seller_id": "1 string", "amount": "2 int64", "bank_code": "3 string",
		"account_number": "4 string", "account_name": "5 string"})
	tcShape(t, fd, "RequestWalletPayoutRequest", map[string]string{"seller_id": "1 string", "amount": "2 int64"})
}

// Scenario: Trust legal contract: Missing internal note is caught
func TestTrustLegal_MissingInternalNoteIsCaught(t *testing.T) {
	fixture := "Internal, service principal with order.internal.read; others get PERMISSION_DENIED."
	if err := tcKeys(fixture, "service principal", "order.internal.read", "PERMISSION_DENIED", "NOT routed at the gateway"); err == nil {
		t.Errorf("helper did not report the missing NOT routed note")
	}
}

// Scenario: Trust legal contract: NOT_SUBMITTED joins the status enum
func TestTrustLegal_NotSubmittedJoinsTheStatusEnum(t *testing.T) {
	fd, _ := tcFile(t, tlVerProto)
	tcEnum(t, fd, "VerificationStatus", map[string]int{
		"VERIFICATION_STATUS_UNSPECIFIED": 0, "VERIFICATION_STATUS_PENDING": 1, "VERIFICATION_STATUS_VERIFIED": 2,
		"VERIFICATION_STATUS_REJECTED": 3, "VERIFICATION_STATUS_NOT_SUBMITTED": 4,
	})
}

// Scenario: Trust legal contract: Verification service lists four new RPCs
func TestTrustLegal_VerificationServiceListsFourNewRPCs(t *testing.T) {
	fd, _ := tcFile(t, tlVerProto)
	tcRPCs(t, fd, "VerificationService", tlRPCMap(
		"SubmitKyc", "GetVerificationStatus", "ReviewKyc",
		"CreateKycUploadUrl", "ListKycSubmissions", "GetKycDocumentUrl", "GetPublicBadges"))
}

// Scenario: Trust legal contract: KYC messages have fixed fields
func TestTrustLegal_KycMessagesHaveFixedFields(t *testing.T) {
	fd, _ := tcFile(t, tlVerProto)
	st := "enum:" + tlVer + "VerificationStatus"
	tcShape(t, fd, "CreateKycUploadUrlRequest", map[string]string{"content_type": "1 string", "size_bytes": "2 int64"})
	tcShape(t, fd, "CreateKycUploadUrlResponse", map[string]string{
		"upload_url": "1 string", "doc_ref": "2 string", "expires_at": "3 " + idTS})
	tcShape(t, fd, "ListKycSubmissionsRequest", map[string]string{"status": "1 " + st, "page": "2 " + idPageReq})
	tcShape(t, fd, "ListKycSubmissionsResponse", map[string]string{
		"submissions": "1 repeated message:" + tlVer + "KycSubmission", "page": "2 " + idPageResp})
	tcShape(t, fd, "KycSubmission", map[string]string{
		"id": "1 string", "user_id": "2 string", "doc_type": "3 string", "status": "4 " + st,
		"document_count": "5 int32", "reason_code": "6 string", "created_at": "7 " + idTS, "reviewed_at": "8 " + idTS})
	tcShape(t, fd, "GetKycDocumentUrlRequest", map[string]string{"submission_id": "1 string", "index": "2 int32"})
	tcShape(t, fd, "GetKycDocumentUrlResponse", map[string]string{"url": "1 string", "expires_at": "2 " + idTS})
	tcShape(t, fd, "GetPublicBadgesRequest", map[string]string{"user_ids": "1 repeated string"})
	tcShape(t, fd, "PublicBadge", map[string]string{"user_id": "1 string", "verified": "2 bool"})
	tcShape(t, fd, "GetPublicBadgesResponse", map[string]string{"badges": "1 repeated message:" + tlVer + "PublicBadge"})
}

// Scenario: Trust legal contract: Existing KYC messages only gain fields
func TestTrustLegal_ExistingKycMessagesOnlyGainFields(t *testing.T) {
	fd, _ := tcFile(t, tlVerProto)
	st := "enum:" + tlVer + "VerificationStatus"
	tcShape(t, fd, "SubmitKycRequest", map[string]string{
		"doc_type": "1 string", "doc_ref": "2 string", "additional_doc_refs": "3 repeated string"})
	tcShape(t, fd, "ReviewKycRequest", map[string]string{"id": "1 string", "decision": "2 string", "reason_code": "3 string"})
	tcShape(t, fd, "GetVerificationStatusResponse", map[string]string{
		"status": "1 " + st, "badge": "2 bool", "reject_reason_code": "3 string"})
	tcShape(t, fd, "SubmitKycResponse", map[string]string{"id": "1 string", "status": "2 " + st})
	tcShape(t, fd, "GetVerificationStatusRequest", map[string]string{"user_id": "1 string"})
	tcShape(t, fd, "ReviewKycResponse", map[string]string{"status": "1 " + st})
}

// Scenario: Trust legal contract: KycSubmission round-trips with unset review time
func TestTrustLegal_KycSubmissionRoundTripsWithUnsetReviewTime(t *testing.T) {
	fd, _ := tcFile(t, tlVerProto)
	md := tcMsg(t, fd, "KycSubmission")
	m := tcNew(t, md, "id", "s-1", "user_id", "u-1", "doc_type", "passport", "status", 1, "document_count", 3,
		"created_at", tcTS(tcFld(t, md, "created_at"), 1760000000))
	got := tcRoundEqual(t, m)
	if got.Has(tcFld(t, md, "reviewed_at")) {
		t.Errorf("unset reviewed_at became set")
	}
	if got.Get(tcFld(t, md, "document_count")).Int() != 3 {
		t.Errorf("document_count changed")
	}
}

// Scenario: Trust legal contract: Status response round-trips NOT_SUBMITTED and a long reason code
func TestTrustLegal_StatusResponseRoundTripsNotSubmittedAndLongReasonCode(t *testing.T) {
	fd, _ := tcFile(t, tlVerProto)
	md := tcMsg(t, fd, "GetVerificationStatusResponse")
	code := strings.Repeat("a", 64)
	got := tcRoundEqual(t, tcNew(t, md, "status", 4, "reject_reason_code", code))
	if got.Get(tcFld(t, md, "status")).Enum() != 4 || got.Get(tcFld(t, md, "reject_reason_code")).String() != code {
		t.Errorf("status or reject_reason_code changed")
	}
}

// Scenario: Trust legal contract: Old readers skip KYC additions and tolerate status 4
func TestTrustLegal_OldReadersSkipKycAdditionsAndTolerateStatus4(t *testing.T) {
	fd, files := tcFile(t, tlVerProto)
	sub := tcNew(t, tcMsg(t, fd, "SubmitKycRequest"), "doc_type", "passport", "doc_ref", "ref-0",
		"additional_doc_refs", []string{"ref-1", "ref-2", "ref-3"})
	st := tcNew(t, tcMsg(t, fd, "GetVerificationStatusResponse"), "status", 4, "reject_reason_code", "blurry")
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropFields(p, "SubmitKycRequest", func(n int32) bool { return n <= 2 })
		tcDropFields(p, "GetVerificationStatusResponse", func(n int32) bool { return n <= 2 })
		tcDropEnumValues(p, "VerificationStatus", func(n int32) bool { return n <= 3 })
	})
	om := old.Messages().ByName("SubmitKycRequest")
	got := tcParseWith(t, sub, om)
	if got.Get(om.Fields().ByName("doc_type")).String() != "passport" || got.Get(om.Fields().ByName("doc_ref")).String() != "ref-0" {
		t.Errorf("old reader lost existing SubmitKycRequest fields")
	}
	sm := old.Messages().ByName("GetVerificationStatusResponse")
	if tcParseWith(t, st, sm).Get(sm.Fields().ByName("status")).Enum() != 4 {
		t.Errorf("status 4 not kept as a number")
	}
}

// Scenario: Trust legal contract: KYC access rules are documented
func TestTrustLegal_KycAccessRulesAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, tlVerProto)
	rc := func(r string) string { return tcRPCComment(t, fd, "VerificationService", r) }
	tcCheck(t, "CreateKycUploadUrl", rc("CreateKycUploadUrl"), "USER principal", "INVALID_ARGUMENT", "5 MiB")
	tcCheck(t, "ListKycSubmissions", rc("ListKycSubmissions"), "admin")
	tcCheck(t, "GetKycDocumentUrl", rc("GetKycDocumentUrl"), "admin", "kyc.document_view", "fail closed", "UNAVAILABLE")
	tcCheck(t, "GetPublicBadges", rc("GetPublicBadges"), "anonymous", "INVALID_ARGUMENT")
	tcCheck(t, "GetVerificationStatus", rc("GetVerificationStatus"), "verification.read")
}

// Scenario: Trust legal contract: Missing audit-first note is caught
func TestTrustLegal_MissingAuditFirstNoteIsCaught(t *testing.T) {
	fixture := "Admin only; fail closed with UNAVAILABLE when the audit store is down; TTL 5 minutes."
	if err := tcKeys(fixture, "admin", "kyc.document_view", "fail closed"); err == nil {
		t.Errorf("helper did not report the missing kyc.document_view note")
	}
}

// Scenario: Trust legal contract: Audit request fields are unchanged
func TestTrustLegal_AuditRequestFieldsAreUnchanged(t *testing.T) {
	fd, _ := tcFile(t, tlAuditProto)
	tcShape(t, fd, "WriteAuditEventRequest", map[string]string{
		"actor_id": "1 string", "action": "2 string", "target_type": "3 string", "target_id": "4 string", "metadata": "5 map"})
}
