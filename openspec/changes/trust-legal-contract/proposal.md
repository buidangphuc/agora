## Why
The trust-legal initiative adds sign-up acceptance of the Terms and Privacy Policy, a server-side consent ledger, account deletion with event-driven erasure, data export, KYC documents with an admin queue, and a KYC gate on payouts. Identity, order, payment, verification, ten consumer services and four frontends need one wire contract first. This slice adds `PrivacyService`, the `RegisterRequest` version fields, `USER_STATUS_DELETED`, the `UserErased` event, the internal obligation RPCs on order and payment, the KYC document RPCs and `NOT_SUBMITTED` on verification, the `kyc.document_view` audit action, and the consumer proto copies. Behaviour lives in the other thirteen slices of the initiative. This slice merges FIRST.

initiative: trust-legal

## What changes
All edits are additive; no existing RPC, message, field or enum value changes or moves. Names, numbers and comments follow the Contracts section of `plans/trust-legal/00-spec.md` (Locked). Canonical source: `platform-core/packages/proto/platform/...`; every consumer file is a byte-identical copy, replaced and never edited by hand.

`platform/identity/v1/identity.proto`:
- `RegisterRequest.terms_version = 5` and `privacy_version = 6` (string). Comment: both required, each must equal the current configured version, else `INVALID_ARGUMENT` and no account.
- `UserStatus.USER_STATUS_DELETED = 5`. Comment: set only by the erasure sweeper; `Login` on a DELETED account fails like an unknown user.
- `service PrivacyService` with `RecordConsent`, `ListMyConsents`, `RequestAccountDeletion`, `CancelAccountDeletion`, `GetAccountDeletionStatus`, `ExportMyData`, plus `enum ConsentPurpose` (values 0-6), `ConsentRecord` and the twelve request/response messages exactly as in the initiative spec. Comment: USER principal only (anonymous `UNAUTHENTICATED`, service principal `PERMISSION_DENIED`), the user id always comes from the principal, routed at the gateway for signed-in users.
- `message UserErased { user_id = 1; repeated string roles = 2; erased_at = 3 (Timestamp) }`. Comment: topic `identity.events`, key = user id, `EventEnvelope.type` "platform.identity.v1.UserErased", written through the identity outbox in the erasure transaction, internal-only, consumers idempotent on `user_id`.

`platform/order/v1/order.proto`: `OrderService.GetUserObligations` with `GetUserObligationsRequest {user_id=1}` and `GetUserObligationsResponse {open_buyer_orders=1, open_seller_orders=2, open_returns=3}` (int32). Comment: internal, SERVICE principal with scope `order.internal.read`, every other principal `PERMISSION_DENIED`, NOT routed at the gateway (501).

`platform/payment/v1/payment.proto`: `PaymentService.GetPayoutObligations` with `GetPayoutObligationsRequest {user_id=1}` and `GetPayoutObligationsResponse {available_balance=1, held_balance=2 (int64), pending_payouts=3 (int32)}`; same internal rules with scope `payment.internal.read`. Comment-only change on `RequestPayout` and `RequestWalletPayout`: `FAILED_PRECONDITION` "seller verification required" unless the seller status is VERIFIED, `UNAVAILABLE` when verification cannot be read.

`platform/verification/v1/verification.proto`:
- `VERIFICATION_STATUS_NOT_SUBMITTED = 4`.
- New RPCs `CreateKycUploadUrl` (USER), `ListKycSubmissions` (admin), `GetKycDocumentUrl` (admin, audited `kyc.document_view` first, fail closed), `GetPublicBadges` (anonymous allowed) with their messages, `KycSubmission` and `PublicBadge`.
- `SubmitKycRequest.additional_doc_refs = 3`, `ReviewKycRequest.reason_code = 3`, `GetVerificationStatusResponse.reject_reason_code = 3`; fields 1-2 are used in each of the three messages, so 3 is the next free number and matches the initiative spec. `verification.proto` gains imports of `google/protobuf/timestamp.proto` and `platform/common/v1/common.proto` (both already present in every consumer tree).
- Comment on `GetVerificationStatus`: a SERVICE principal with scope `verification.read` may read any `user_id`.

`platform/audit/v1/audit.proto`: the action catalogue comment grows from 25 to 26 actions by adding `kyc.document_view`. `platform-core/packages/proto/gaps_contract_docs_test.go` pins "Action catalogue (25)" and moves to 26 in this slice.

Copies (byte-identical to the source):
- `identity.proto`, `order.proto`, `payment.proto` into every existing copy: team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-notification, team-order, team-payment, team-promotion, team-referral, team-search (`identity.proto` also into team-identity).
- `verification.proto` into team-analytics, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-verification.
- `audit.proto` into team-analytics, team-audit, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-identity, team-order, team-payment, team-promotion, team-verification.
- New copies so the consumer compiles: team-verification gets `identity/v1/identity.proto` and `events/v1/events.proto` (it consumes `identity.events/UserErased`).

A committed Go test `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` pins the contract (dynamic descriptor from `buf build`, as `promotion/v1/flashstock_compat_test.go` does).

Out of scope: any runtime behaviour (consent storage, sweeper, obligation counting, presigned URLs, gate, UI), gateway routes and 501 entries, scope grants in identity, MinIO bucket, renumbering any existing field.

## NFR
- Additive only: `buf breaking` passes against the base from main. Old readers skip the new fields and parse unknown enum values as numbers.
- No new service, port or dependency. Contract-only: no runtime latency or availability impact. Initiative runtime NFRs (`RecordConsent` p95 50 ms, `ExportMyData` p95 500 ms and 1 MiB, payout gate 2 s, presigned TTL 15 min PUT / 5 min GET, erasure within 1 h of `UserErased`) are owned by the behaviour slices; this slice documents them in RPC comments.
- Limits fixed by the contract: `RecordConsent` 1..6 records, `GetPublicBadges` 1..50 ids, upload 1..5 MiB, 0..2 additional doc refs, `ConsentRecord.version` at most 32 chars, `reason_code` lower_snake_case at most 64 chars.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `ConsentRecord` (purpose, granted, version, recorded_at, source) | personal (consent proof, linked to the user id) | prove consent and show history | legal obligation, Decree 13/2023 Art. 9-11 | account life plus N years after erasure, N set by legal (NEEDS_HUMAN) |
| `RegisterRequest.terms_version`, `privacy_version` | non-personal document versions | record what was accepted | contract and consent proof | stored as consent records |
| `RequestAccountDeletionRequest.password` | credential, sensitive | re-authenticate the deletion | security | never stored or logged; checked in memory only |
| `GetAccountDeletionStatusResponse.erase_after` | personal (request timestamp) | data-subject request handling | legal obligation | deleted with the erasure, outcome stays as `USER_STATUS_DELETED` |
| `ExportMyDataResponse.data` | personal (full account export) | data-subject access right | legal obligation | never stored server-side, `Cache-Control: no-store` |
| `UserErased.user_id`, `roles`, `erased_at` | pseudonymous id (identifier of an erased person) | drive erasure in consumers | legal obligation | `identity.events` topic retention |
| `GetUserObligations`, `GetPayoutObligations` payloads | business data tied to a user id | block deletion while obligations are open | legal and contract | not stored by the callee for this purpose |
| `CreateKycUploadUrlResponse.doc_ref`, `KycSubmission.document_count` | pointer to a sensitive ID image | locate stored ID documents | seller identification duty (Decree 85/2021), explicit consent checkbox at upload | VERIFIED kept while the seller account exists; REJECTED or superseded deleted after 90 days (DPO sign-off NEEDS_HUMAN) |
| `GetKycDocumentUrlResponse.url` | short-lived signed URL to an ID image | admin review | legal duty, audited | 5-min TTL, never logged |
| `PublicBadge.verified` | boolean derived from KYC | shop badge | legitimate interest, no other status exposed | derived on read |

## Threat model
- Assets: ID document images and signed URLs, the deletion trigger, consent history, user obligations (balances), payout eligibility.
- Abuse cases: another user or a service triggers deletion or reads consents (user id taken from the principal only, services `PERMISSION_DENIED`); a stolen session deletes the account (password re-entry in the contract); a browser or anonymous caller reads obligations (internal scope, 501 at the edge); a user submits a `doc_ref` issued to someone else (comment requires issued-to-caller); an admin views an ID without a trace (`kyc.document_view` audited first, fail closed); enumeration of verification status through the public badge RPC (boolean only, 1..50 ids); payout while unverified or while verification is unreachable (fail closed).
- Authz rule: every `PrivacyService` RPC is a USER principal acting on itself; obligation RPCs need a SERVICE principal with an internal read scope; KYC list, document URL and review are admin; `GetPublicBadges` is anonymous and returns only `verified`. Comment text states each rule so behaviour slices implement and test it; this slice enforces nothing at runtime.
