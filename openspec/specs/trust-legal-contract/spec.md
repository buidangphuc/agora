# trust-legal-contract Specification

## Purpose
TBD - created by archiving change trust-legal-contract. Update Purpose after archive.

## Requirements

### Requirement: Identity carries legal versions, the DELETED status and PrivacyService

`RegisterRequest` in `platform/identity/v1/identity.proto` SHALL gain `string terms_version = 5` and `string privacy_version = 6` with fields 1-4 unchanged. `UserStatus` SHALL gain `USER_STATUS_DELETED = 5` with values 0-4 unchanged. The file SHALL define `service PrivacyService` with exactly `RecordConsent`, `ListMyConsents`, `RequestAccountDeletion`, `CancelAccountDeletion`, `GetAccountDeletionStatus` and `ExportMyData`, `enum ConsentPurpose` with `CONSENT_PURPOSE_UNSPECIFIED=0`, `_ANALYTICS=1`, `_MARKETING=2`, `_PERSONALIZATION=3`, `_TERMS=4`, `_PRIVACY_POLICY=5`, `_SELLER_TERMS=6`, and `ConsentRecord {purpose=1, granted=2 (bool), version=3 (string), recorded_at=4 (Timestamp), source=5 (string)}`. The request and response messages SHALL be `RecordConsentRequest {repeated ConsentRecord records=1}`, `RecordConsentResponse {}`, `ListMyConsentsRequest {platform.common.v1.PageRequest page=1}`, `ListMyConsentsResponse {repeated ConsentRecord records=1, platform.common.v1.PageResponse page=2}`, `RequestAccountDeletionRequest {string password=1}`, `RequestAccountDeletionResponse {Timestamp erase_after=1}`, `CancelAccountDeletionRequest {}`, `CancelAccountDeletionResponse {}`, `GetAccountDeletionStatusRequest {}`, `GetAccountDeletionStatusResponse {bool pending=1, Timestamp erase_after=2}`, `ExportMyDataRequest {}` and `ExportMyDataResponse {bytes data=1, string content_type=2, Timestamp generated_at=3}`. The comments SHALL state: both versions are required and must equal the current configured version, else `INVALID_ARGUMENT` with no account; `USER_STATUS_DELETED` is set only by the erasure sweeper and `Login` on it fails like an unknown user; `PrivacyService` accepts a USER principal only (anonymous `UNAUTHENTICATED`, service principal `PERMISSION_DENIED`), the user id always comes from the principal and no request field selects a user, and it is routed at the gateway for signed-in users; `RecordConsent` takes 1 to 6 records, one per purpose, `recorded_at` is server-set and ignored on input, `version` is at most 32 characters; `RequestAccountDeletion` requires the password and fails `PERMISSION_DENIED` on a wrong password and `FAILED_PRECONDITION` naming the blocker when obligations are open; `ExportMyData` returns at most 1 MiB and is never stored.

#### Scenario: Trust legal contract: RegisterRequest carries both legal versions
- **WHEN** the descriptor test reads `RegisterRequest`
- **THEN** fields 1 to 4 equal the committed expectations and `terms_version`=5 and `privacy_version`=6 are strings
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: UserStatus gains DELETED without moving values
- **WHEN** the descriptor test reads `UserStatus`
- **THEN** values 0 to 4 are unchanged and `USER_STATUS_DELETED` equals 5
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: PrivacyService has six RPCs and fixed messages
- **WHEN** the descriptor test reads `PrivacyService` and its messages
- **THEN** it lists exactly the six RPCs with the named request and response types and every message field has the stated name, type and number
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: ConsentPurpose has fixed names and numbers
- **WHEN** the descriptor test reads `ConsentPurpose`
- **THEN** it has exactly the seven values with numbers 0 to 6
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: ConsentRecord round-trips with a 32-character version
- **WHEN** a `ConsentRecord` with `granted` false, a 32-character `version` and a `recorded_at` is serialised and parsed
- **THEN** every field parses back equal, and an unset `recorded_at` parses back unset
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: ExportMyData response round-trips a 1 MiB payload
- **WHEN** an `ExportMyDataResponse` with 1048576 bytes of data is serialised and parsed
- **THEN** `data` parses back byte-equal with its `content_type` and `generated_at`
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Old readers skip the register versions
- **WHEN** bytes of a `RegisterRequest` carrying fields 5 and 6 are parsed with a schema that knows only fields 1-4
- **THEN** parsing succeeds, fields 1-4 are intact and fields 5 and 6 are skipped without error
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Old readers parse an unknown status as a number
- **WHEN** bytes carrying `UserStatus` value 5 are parsed with a schema that stops at 4
- **THEN** parsing succeeds and the value is kept as the number 5
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Privacy principal rules are documented
- **WHEN** a reader opens the comments of `PrivacyService`, `RequestAccountDeletion` and `ExportMyData` to learn who may call them
- **THEN** they contain "USER principal", "UNAUTHENTICATED", "PERMISSION_DENIED", "no request field", "FAILED_PRECONDITION", "password" and "1 MiB"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Register and DELETED semantics are documented
- **WHEN** a reader opens the comments of `RegisterRequest.terms_version`, `privacy_version` and `USER_STATUS_DELETED`
- **THEN** they contain "INVALID_ARGUMENT", "current configured version", "erasure sweeper" and "unknown user"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Missing principal note is caught
- **WHEN** the comment helper runs against a fixture `PrivacyService` comment lacking "USER principal"
- **THEN** the helper reports failure (the test expects it), proving a missing note is caught
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

### Requirement: Identity defines the UserErased event

`platform/identity/v1/identity.proto` SHALL define `message UserErased {string user_id=1, repeated string roles=2, google.protobuf.Timestamp erased_at=3}`. Its comment SHALL state: published on `identity.events`, key = user id, `EventEnvelope.type` "platform.identity.v1.UserErased", written through the identity outbox in the erasure transaction, internal-only (never exposed at the gateway), consumers idempotent on `user_id`, `roles` are the roles held at erasure.

#### Scenario: Trust legal contract: UserErased message is fixed
- **WHEN** the descriptor test reads `UserErased`
- **THEN** it has exactly `user_id`=1 string, `roles`=2 repeated string, `erased_at`=3 `google.protobuf.Timestamp`
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: UserErased round-trips with several roles
- **WHEN** a `UserErased` with a 26-character `user_id`, roles `buyer` and `seller` and an `erased_at` is serialised and parsed
- **THEN** every field parses back equal with the roles in order
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: UserErased without roles round-trips
- **WHEN** a `UserErased` with an empty `roles` list is serialised and parsed
- **THEN** `roles` parses back empty and `user_id` is intact
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: UserErased semantics are documented
- **WHEN** a reader opens the comment of `UserErased`
- **THEN** it contains `identity.events`, "platform.identity.v1.UserErased", "outbox", "same transaction", "idempotent" and "internal-only"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: UserErased is documented as not exposed to any principal at the edge
- **WHEN** a reader opens the comment of `UserErased` to learn whether a user principal may receive it
- **THEN** it contains "never exposed at the gateway"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Missing idempotency note is caught
- **WHEN** the comment helper runs against a fixture `UserErased` comment lacking "idempotent"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

### Requirement: Order and payment expose internal obligation RPCs

`OrderService` in `platform/order/v1/order.proto` SHALL gain `rpc GetUserObligations(GetUserObligationsRequest) returns (GetUserObligationsResponse)` with `GetUserObligationsRequest {string user_id=1}` and `GetUserObligationsResponse {int32 open_buyer_orders=1, int32 open_seller_orders=2, int32 open_returns=3}`. `PaymentService` in `platform/payment/v1/payment.proto` SHALL gain `rpc GetPayoutObligations(GetPayoutObligationsRequest) returns (GetPayoutObligationsResponse)` with `GetPayoutObligationsRequest {string user_id=1}` and `GetPayoutObligationsResponse {int64 available_balance=1, int64 held_balance=2, int32 pending_payouts=3}`. Both comments SHALL state: internal, service-to-service only, SERVICE principal with scope `order.internal.read` or `payment.internal.read` respectively, USER, anonymous and admin principals get `PERMISSION_DENIED`, NOT routed at the gateway (501). The comments of `RequestPayout` and `RequestWalletPayout` SHALL state `FAILED_PRECONDITION` "seller verification required" unless the seller status is VERIFIED and `UNAVAILABLE` when verification cannot be read; no field of either request or response changes.

#### Scenario: Trust legal contract: GetUserObligations exists with fixed messages
- **WHEN** the descriptor test reads `OrderService`
- **THEN** every existing RPC is unchanged and `GetUserObligations` has request `user_id`=1 string and response fields 1 to 3 of type int32
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: GetPayoutObligations exists with fixed messages
- **WHEN** the descriptor test reads `PaymentService`
- **THEN** every existing RPC is unchanged and `GetPayoutObligations` has request `user_id`=1 string and response `available_balance`=1 int64, `held_balance`=2 int64, `pending_payouts`=3 int32
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Obligation responses round-trip extreme values
- **WHEN** a `GetPayoutObligationsResponse` with `available_balance` 9223372036854775807 and a `GetUserObligationsResponse` with all counts zero are serialised and parsed
- **THEN** the first parses back equal and the second parses back as the empty message
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Obligation RPCs are documented as internal
- **WHEN** a reader opens the comments of both RPCs to learn who may call them
- **THEN** they contain "service principal", the scope name, "PERMISSION_DENIED" and "NOT routed at the gateway"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Payout gate wording is documented
- **WHEN** a reader opens the comments of `RequestPayout` and `RequestWalletPayout`
- **THEN** they contain "seller verification required", "VERIFIED", "FAILED_PRECONDITION" and "UNAVAILABLE"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Payout request fields are unchanged
- **WHEN** the descriptor test reads `RequestPayoutRequest` and `RequestWalletPayoutRequest`
- **THEN** their fields equal the committed pre-change expectations
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Missing internal note is caught
- **WHEN** the comment helper runs against a fixture `GetUserObligations` comment lacking "NOT routed at the gateway"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

### Requirement: Verification gains KYC document RPCs and NOT_SUBMITTED

`VerificationStatus` in `platform/verification/v1/verification.proto` SHALL gain `VERIFICATION_STATUS_NOT_SUBMITTED = 4` with values 0-3 unchanged. `VerificationService` SHALL gain `CreateKycUploadUrl`, `ListKycSubmissions`, `GetKycDocumentUrl` and `GetPublicBadges` with `CreateKycUploadUrlRequest {string content_type=1, int64 size_bytes=2}`, `CreateKycUploadUrlResponse {string upload_url=1, string doc_ref=2, Timestamp expires_at=3}`, `ListKycSubmissionsRequest {VerificationStatus status=1, platform.common.v1.PageRequest page=2}`, `ListKycSubmissionsResponse {repeated KycSubmission submissions=1, platform.common.v1.PageResponse page=2}`, `KycSubmission {string id=1, string user_id=2, string doc_type=3, VerificationStatus status=4, int32 document_count=5, string reason_code=6, Timestamp created_at=7, Timestamp reviewed_at=8}`, `GetKycDocumentUrlRequest {string submission_id=1, int32 index=2}`, `GetKycDocumentUrlResponse {string url=1, Timestamp expires_at=2}`, `GetPublicBadgesRequest {repeated string user_ids=1}`, `PublicBadge {string user_id=1, bool verified=2}` and `GetPublicBadgesResponse {repeated PublicBadge badges=1}`. `SubmitKycRequest` SHALL gain `repeated string additional_doc_refs = 3`, `ReviewKycRequest` SHALL gain `string reason_code = 3` and `GetVerificationStatusResponse` SHALL gain `string reject_reason_code = 3`, with all existing fields unchanged. The comments SHALL state: `CreateKycUploadUrl` is USER principal only and accepts content type image/jpeg, image/png or application/pdf with size 1 to 5 MiB, else `INVALID_ARGUMENT`; `ListKycSubmissions` is admin only and returns oldest first; `GetKycDocumentUrl` is admin only, writes the `kyc.document_view` audit entry first and fails closed with `UNAVAILABLE` when the audit store is down, index 0 is the `doc_ref`, TTL 5 minutes; `GetPublicBadges` allows anonymous callers, takes 1 to 50 ids else `INVALID_ARGUMENT` and exposes no status other than the boolean; `SubmitKyc` requires every doc ref issued to the caller and uploaded, else `INVALID_ARGUMENT`, with 0 to 2 additional refs; `ReviewKyc` requires `reason_code` (lower_snake_case, at most 64 characters) on reject and empty on approve; a user with no submission reads `NOT_SUBMITTED`; `GetVerificationStatus` accepts a SERVICE principal with scope `verification.read` for any `user_id`, and `reject_reason_code` is shown only in the own or admin view.

#### Scenario: Trust legal contract: NOT_SUBMITTED joins the status enum
- **WHEN** the descriptor test reads `VerificationStatus`
- **THEN** values 0 to 3 are unchanged and `VERIFICATION_STATUS_NOT_SUBMITTED` equals 4
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Verification service lists four new RPCs
- **WHEN** the descriptor test reads `VerificationService`
- **THEN** the three existing RPCs are unchanged and the four new RPCs exist with the named request and response types
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: KYC messages have fixed fields
- **WHEN** the descriptor test reads the new KYC messages
- **THEN** every field has the stated name, type and number and `KycSubmission` has exactly eight fields
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Existing KYC messages only gain fields
- **WHEN** the descriptor test reads `SubmitKycRequest`, `ReviewKycRequest` and `GetVerificationStatusResponse`
- **THEN** existing fields keep their numbers and the three new fields equal 3
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: KycSubmission round-trips with unset review time
- **WHEN** a `KycSubmission` with `document_count` 3 and unset `reviewed_at` is serialised and parsed
- **THEN** it parses back equal with `reviewed_at` unset
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Status response round-trips NOT_SUBMITTED and a long reason code
- **WHEN** a `GetVerificationStatusResponse` with status 4 and a 64-character `reject_reason_code` is serialised and parsed
- **THEN** both parse back equal
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Old readers skip KYC additions and tolerate status 4
- **WHEN** bytes of a `SubmitKycRequest` with three `additional_doc_refs` and a status value 4 are parsed with the pre-change schema
- **THEN** parsing succeeds, existing fields are intact, field 3 is skipped and the status is kept as the number 4
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: KYC access rules are documented
- **WHEN** a reader opens the comments of the four new RPCs and `GetVerificationStatus`
- **THEN** they contain "USER principal", "admin", "kyc.document_view", "fail closed", "anonymous", "verification.read", "INVALID_ARGUMENT" and "5 MiB"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

#### Scenario: Trust legal contract: Missing audit-first note is caught
- **WHEN** the comment helper runs against a fixture `GetKycDocumentUrl` comment lacking "kyc.document_view"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

### Requirement: Audit catalogue lists kyc.document_view

The `WriteAuditEventRequest` comment in `platform/audit/v1/audit.proto` SHALL carry the header "Action catalogue (26)" and list `kyc.document_view` in addition to the 25 existing actions. `platform-core/packages/proto/gaps_contract_docs_test.go` SHALL pin the header at 26. No field of `WriteAuditEventRequest` changes.

#### Scenario: Trust legal contract: Catalogue header counts 26 actions
- **WHEN** the docs test reads the `WriteAuditEventRequest` comment
- **THEN** it contains "Action catalogue (26)" and all 26 action names including `kyc.document_view`
- **VERIFIED BY**: Go test `platform-core/packages/proto/gaps_contract_docs_test.go` (tasks 1.5)

#### Scenario: Trust legal contract: Stale catalogue header is rejected
- **WHEN** the docs helper runs against a fixture comment with "Action catalogue (25)"
- **THEN** the helper reports a problem, proving a stale count is caught
- **VERIFIED BY**: Go test `platform-core/packages/proto/gaps_contract_docs_test.go` (tasks 1.5)

#### Scenario: Trust legal contract: Audit request fields are unchanged
- **WHEN** the descriptor test reads `WriteAuditEventRequest`
- **THEN** fields 1 to 5 equal the pre-change expectations
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (tasks 2.1)

### Requirement: The change is backward compatible and consumers carry identical copies

Every edit SHALL be additive so that `buf breaking` against the base from main passes and `buf lint` passes. Consumer repos SHALL carry byte-identical copies of the changed protos: `identity.proto`, `order.proto`, `payment.proto` and `audit.proto` where they already exist, `verification.proto` in its six consumers, plus new `identity.proto` and `events.proto` copies in team-verification. A missing or drifted copy SHALL make the copy check fail. Generated code SHALL contain the new symbols in every language `make proto` targets.

#### Scenario: Trust legal contract: Change is backward compatible
- **WHEN** `make -C platform-core breaking` runs against the base
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto breaking` (tasks 1.6)

#### Scenario: Trust legal contract: Wire-breaking edit is rejected
- **WHEN** a scratch copy renumbers one new field and retypes one existing field and `make -C platform-core breaking` runs
- **THEN** it exits non-zero
- **VERIFIED BY**: `make -C platform-core breaking` on a mutated scratch copy (tasks 1.6)

#### Scenario: Trust legal contract: Lint passes
- **WHEN** `buf lint` runs over the proto tree
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto` (tasks 1.6)

#### Scenario: Trust legal contract: Copies match the source
- **WHEN** the copy script compares each listed source and consumer path with `cmp`
- **THEN** every pair is identical
- **VERIFIED BY**: shell test `platform-core/packages/proto/trust_legal_consumer_copies_test.sh` (tasks 3.1)

#### Scenario: Trust legal contract: Drifted or missing copy is rejected
- **WHEN** the copy script runs on a scratch tree with one changed byte and one deleted copy
- **THEN** it exits non-zero and names both files
- **VERIFIED BY**: shell test `platform-core/packages/proto/trust_legal_consumer_copies_test.sh` (tasks 3.1)

#### Scenario: Trust legal contract: Consumer protos build
- **WHEN** `buf build` runs in `team-verification/proto` and in each consumer proto directory with a `buf.yaml`
- **THEN** every build exits 0
- **VERIFIED BY**: shell test `platform-core/packages/proto/trust_legal_consumer_copies_test.sh` (tasks 3.2)

#### Scenario: Trust legal contract: New contract in every generated language
- **WHEN** `make -C platform-core proto` runs and the Go, Python, JVM and TS outputs are searched for `PrivacyService`, `UserErased`, `GetUserObligations`, `GetPayoutObligations`, `CreateKycUploadUrl` and `GetPublicBadges`
- **THEN** each symbol is found in each language
- **VERIFIED BY**: grep over generated output (tasks 2.2)
