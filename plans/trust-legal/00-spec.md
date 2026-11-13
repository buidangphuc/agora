# 00-spec — trust & legal: data-subject rights, legal pages, KYC-gated payouts

initiative: trust-legal

## Goal
A user can read the platform's legal documents in Vietnamese, accepts the current Terms and Privacy
Policy at sign-up (recorded server-side with version and time), can see and change their consent
history, download their account data, and delete their account so every service erases or
pseudonymises their personal data. A seller uploads real identity documents, an admin works them
from a KYC queue, and no payout leaves the platform for a seller who is not `VERIFIED`.
Backlog: Q24 (trust#10) and Q28 phase 1 (trust#4) of the 2026-10-14 PM review.

## Current state (verified on 06c44c17)
- No deletion, export or consent RPC: `identity.proto:14-66` (AuthService), `:338`, `:370-382`.
  Only seller terms are recorded: `BecomeSellerRequest.terms_version` (`identity.proto:103-105`),
  `team-identity/internal/store/migrations/0021_seller_terms.up.sql:2-3`; `RegisterRequest`
  (`identity.proto:111-123`) has no terms field. The seller onboarding "terms" link goes to the
  buyer home page (`team-frontend-seller/src/features/seller/SellerOnboarding.tsx:49`).
- No legal routes in any app (`find team-frontend*/src/app -name page.tsx`); the footer "Chính Sách
  Trả Hàng" links to `/account/orders` (`team-frontend/src/components/shell/ConsumerShell.tsx:329-335`).
  The consent banner labels are English (`team-frontend/src/features/consent/ConsentBanner.tsx:14-28,127`)
  and consent lives only in a cookie (`team-frontend/src/lib/consent/consent.ts`, read by
  `team-gateway/internal/edge/consent_cookie.go`).
- Q2 left for Q24: erasure of `reviews.hidden_by`/`hide_reason`
  (`team-engagement/migrations/0011_review_integrity.up.sql:5-6`) and of team-audit copies, which
  are append-only (`team-audit/internal/store/migrations/0001_create_audit_events.up.sql:1`).
- KYC: `doc_ref` is free text typed by the user (`team-frontend/src/features/account/verification/SubmitKycForm.tsx:26,33`),
  stored as-is (`team-verification/migrations/001_create_kyc_submissions.up.sql:5-14`); no list RPC
  (`verification.proto:6-13`); "no submission" reads as `PENDING` (`team-verification/internal/verification/verification.go:55-57`).
  Nothing consults KYC: `RequestPayout`/`RequestWalletPayout` check only ownership and `listing.write`
  (`team-payment/internal/rpc/payment.go:230-249`, `wallet_ledger.go:38-46,84-98`); no
  verification reference in team-payment/domain/order. Admin `/admin/cases` has reports, moderation
  and disputes tabs but no KYC tab (`team-frontend-admin/src/app/admin/cases/page.tsx:6-17`).
- Already done, not in scope: `ReviewKyc` is audited fail-closed and blocks self-review
  (`team-verification/README.md` Contract table); user enforcement (`SetUserStatus`) and status-narrowed
  scopes exist (`team-identity/internal/authz/scopes.go:84-99`).

## User stories
1. **Accept legal docs at sign-up.** Given the current Terms version T and Privacy version P, when a
   visitor registers with both versions, then the account exists and two consent records (T, P, time)
   are stored. Edge: a missing or stale version → `INVALID_ARGUMENT`, no account. Edge: when T changes,
   a signed-in user whose latest accepted Terms is older sees a re-acceptance prompt.
2. **Consent ledger.** Given a signed-in user changes analytics/marketing/personalization in the banner,
   when they save, then the cookie is set and one record per changed purpose is stored; `/account/privacy`
   lists the history newest first. Edge: anonymous visitors keep cookie-only consent; server-side records exist for signed-in users only.
3. **Delete my account.** Given a buyer with no open obligations, when they confirm deletion with their
   password, then deletion is scheduled for `erase_after`, all their sessions except the current one are
   revoked, and they can cancel until then; after `erase_after` identity erases its own data, publishes
   `UserErased`, and each consuming service erases or pseudonymises the user's data idempotently.
   Edge: open orders/returns or a non-zero wallet balance → `FAILED_PRECONDITION` naming the blocker; deletion is also blocked while payouts are pending, and the reason is shown to the user.
   Edge: wrong password → `PERMISSION_DENIED`, nothing scheduled.
4. **Export my data.** Given a signed-in user, when they request an export, then they download one JSON
   file with their identity-held data and consent history.
5. **Seller uploads KYC.** Given a seller, when they upload 1-3 images/PDFs (≤ 5 MiB each) of an ID document
   through presigned URLs and submit, then a `PENDING` submission exists; a doc_ref not issued to the
   caller is rejected. Edge: a seller who never submitted sees `NOT_SUBMITTED`.
6. **Admin KYC queue.** Given pending submissions, when an admin opens the KYC tab, then they page
   through them oldest first, open each document through a 5-minute URL (each view audited
   `kyc.document_view`, fail closed), and approve or reject with a reason code.
7. **Payout gate.** Given a seller whose latest submission is not `VERIFIED`, when they request a payout,
   then the call fails `FAILED_PRECONDITION` and no ledger row is written. Edge: team-verification
   unreachable → `UNAVAILABLE`, no payout (fail closed).
8. **Public badge.** Given a verified seller, when anyone views their shop page, then a "Đã xác minh" badge
   shows; no status other than the boolean badge is ever exposed publicly.

## Non-goals
- Real eKYC vendor (VNPT/FPT.AI), CCCD NFC chip read, liveness — Q28 phase 2, later initiative.
- Gating listing creation/publishing on KYC: KYC gates payouts only.
- Bank-account holder name matching against the KYC identity: deferred; KYC does not capture the legal full name for matching in this initiative.
- Admin MFA and splitting the `admin` role (trust#10 tail) — a separate security initiative.
- Writing legal text: the legal owner fills every placeholder; engineering ships structure and versions only.
- Staging/prod work (Vault keys, gitops bucket, retention cron in k8s) — local first.

## Architecture decisions
- team-identity owns the consent ledger, legal-version acceptance and the deletion lifecycle, in a new
  `PrivacyService` on its existing gRPC server. Why: it owns the user row and already records seller terms.
- Current versions come from config `LEGAL_TERMS_VERSIONS` / `PRIVACY_POLICY_VERSIONS` (comma list, first =
  current), mirroring `SELLER_TERMS_VERSIONS` (`team-identity/internal/config/config.go:147-158`); the frontends
  read the same version from `NEXT_PUBLIC_*` like `SELLER_TERMS_VERSION`.
- Deletion: `RequestAccountDeletion` sets `deletion_requested_at`/`erase_after`; a sweeper in team-identity
  (mirror `internal/identity/session_sweeper.go`) erases due accounts: anonymises username to
  `deleted_<random>`, clears password hash, deletes addresses, sessions, login history and handoff codes, keeps
  consent records and seller-terms acceptance as proof, sets status `deleted` (login refused), and writes
  `UserErased` to the identity outbox in the same transaction. The cancel window is 48 h (`erase_after` = request + 48 h) and erasure completes within 72 h of the request
  (the Decree 13/2023 Art. 16 "72 h" figure is unverified; see NEEDS_HUMAN).
- Erasure fan-out is event-driven: each service consumes `identity.events/UserErased` with its own consumer
  group and DLQ (`identity.events.<svc>.dlq`), idempotent by user id; no service calls another's DB.
  Per-service rules:
  - team-engagement: reviews and Q&A keep rating/text with author shown as "Người dùng đã xóa"; `hide_reason`
    cleared on the user's reviews; `hidden_by` cleared where it equals the user; favorites, follows, reports
    filed by the user deleted; the text is kept, only the author is anonymised.
  - team-chat / team-notification: notifications deleted; the user's chat message text replaced by a fixed
    placeholder, thread kept for the counterparty.
  - team-order / team-payment / team-promotion / team-referral: orders, ledger, payouts, redemptions kept for
    accounting with ids and amounts; recipient name, phone, address and payout `account_number`/`account_name`
    replaced by "Đã xóa"; ids and amounts are kept for the accounting retention period (length set by legal, NEEDS_HUMAN) and bank details are masked, not kept.
  - team-domain: an erased seller's listings are deleted through the existing delete path (search, analytics
    and team-ai react to the existing `ListingChanged` DELETED); listings are not kept as archived.
  - team-analytics / platform-recsys / platform-featurestore / team-ai: rows keyed by the user id are deleted
    or re-keyed to a non-reversible id; the next model/feature build drops them.
  - team-verification: submissions and stored documents of the user deleted, subject to the KYC retention rule (see NEEDS_HUMAN and Personal data).
  - team-audit: unchanged; the immutable copies (user ids, free-text reasons) are kept under legal obligation, pending DPO sign-off (NEEDS_HUMAN).
- Obligation check at request time is synchronous and fail closed: team-identity calls the new internal
  `OrderService.GetUserObligations` and `PaymentService.GetPayoutObligations` with its service principal
  (scopes `order.internal.read`, `payment.internal.read`); both are not routed at the gateway (501).
- Re-authentication for deletion: the request carries the password, checked like `ChangePassword`; password re-entry is required.
- Export v1 is synchronous from team-identity only (account, addresses, sessions, login history, consents,
  deletion state) as JSON ≤ 1 MiB; the export page links the in-app views of orders, reviews and chats;
  there is no fan-out across other services.
- KYC documents: team-verification gets its own private MinIO bucket `kyc-documents` (separate from
  `listing-images`) with server-side encryption, presigned PUT (content-type and length bound in the signature,
  15-min TTL, key `kyc/<user>/<uuid>`), and presigned GET (5-min TTL) only via the audited admin RPC. The
  service checks the object exists and belongs to the caller before accepting `SubmitKyc`. Why: never
  stream ID images through services; access is audited (ASVS V8.3).
- `GetVerificationStatus` stays as is for users/admins and additionally accepts a service principal with
  scope `verification.read`; team-payment calls it (≤ 2 s, single attempt) on both payout RPCs and fails
  closed. Why: money needs the fresh status, not a JWT claim that may be stale until refresh.
- A user with no submission reads `NOT_SUBMITTED` (new enum value); the badge is still `status == VERIFIED`.
- KYC moves to the seller app (`/seller/verification`); the buyer `/account/verification` route is removed;
  KYC exists only in the seller app.
- Legal pages are static Server Components in team-frontend under Vietnamese slugs, rendered from
  `content/legal/<slug>.md` with a version header; every body is a placeholder block "NỘI DUNG DO BỘ PHẬN PHÁP
  CHẾ CUNG CẤP" until the legal owner supplies it. The page set is seven pages with these slugs: dieu-khoan-su-dung, chinh-sach-bao-mat, quy-che-hoat-dong, chinh-sach-doi-tra, hang-cam-kinh-doanh, giai-quyet-khieu-nai, dieu-khoan-nguoi-ban. The footer carries
  the operator block required for e-commerce platforms as placeholders (company name, business registration number, address, Ministry of Industry and Trade registration notice); the legal owner fills them (NEEDS_HUMAN).
- New dependency: MinIO client in team-verification (same SDK as team-domain `internal/objects/storage.go`). No other new dependency.

## Contracts
```proto
// platform/identity/v1/identity.proto
// RegisterRequest: both required, must equal the current configured version, else INVALID_ARGUMENT.
string terms_version = 5;
string privacy_version = 6;
// UserStatus: set only by the erasure sweeper; Login on a DELETED account fails like an unknown user.
USER_STATUS_DELETED = 5;

// PrivacyService: USER principal only (anonymous UNAUTHENTICATED, service PERMISSION_DENIED); the user id
// always comes from the principal. Routed at the gateway for signed-in users.
service PrivacyService {
  rpc RecordConsent(RecordConsentRequest) returns (RecordConsentResponse);
  rpc ListMyConsents(ListMyConsentsRequest) returns (ListMyConsentsResponse);
  rpc RequestAccountDeletion(RequestAccountDeletionRequest) returns (RequestAccountDeletionResponse);
  rpc CancelAccountDeletion(CancelAccountDeletionRequest) returns (CancelAccountDeletionResponse);
  rpc GetAccountDeletionStatus(GetAccountDeletionStatusRequest) returns (GetAccountDeletionStatusResponse);
  rpc ExportMyData(ExportMyDataRequest) returns (ExportMyDataResponse);
}
enum ConsentPurpose {
  CONSENT_PURPOSE_UNSPECIFIED = 0; CONSENT_PURPOSE_ANALYTICS = 1; CONSENT_PURPOSE_MARKETING = 2;
  CONSENT_PURPOSE_PERSONALIZATION = 3; CONSENT_PURPOSE_TERMS = 4; CONSENT_PURPOSE_PRIVACY_POLICY = 5;
  CONSENT_PURPOSE_SELLER_TERMS = 6;
}
message ConsentRecord {
  ConsentPurpose purpose = 1;
  bool granted = 2;
  string version = 3;                          // document or banner version, ≤ 32 chars
  google.protobuf.Timestamp recorded_at = 4;   // server-set; ignored on input
  string source = 5;                           // "register" | "banner" | "account" | "seller_onboarding"
}
message RecordConsentRequest { repeated ConsentRecord records = 1; }   // 1..6, one per purpose
message RecordConsentResponse {}
message ListMyConsentsRequest { platform.common.v1.PageRequest page = 1; }
message ListMyConsentsResponse { repeated ConsentRecord records = 1; platform.common.v1.PageResponse page = 2; }
message RequestAccountDeletionRequest { string password = 1; }
message RequestAccountDeletionResponse { google.protobuf.Timestamp erase_after = 1; }
message CancelAccountDeletionRequest {}
message CancelAccountDeletionResponse {}
message GetAccountDeletionStatusRequest {}
message GetAccountDeletionStatusResponse { bool pending = 1; google.protobuf.Timestamp erase_after = 2; }
message ExportMyDataRequest {}
message ExportMyDataResponse { bytes data = 1; string content_type = 2; google.protobuf.Timestamp generated_at = 3; }

// Event on Kafka `identity.events` (key = user id, EventEnvelope type "platform.identity.v1.UserErased"),
// written through the identity outbox in the erasure transaction. Consumers must be idempotent on user_id.
message UserErased {
  string user_id = 1;
  repeated string roles = 2;                    // roles held at erasure, so sellers' data is handled
  google.protobuf.Timestamp erased_at = 3;
}

// platform/order/v1/order.proto — add to OrderService. Internal: service principal with
// order.internal.read; not routed at the gateway (501).
rpc GetUserObligations(GetUserObligationsRequest) returns (GetUserObligationsResponse);
message GetUserObligationsRequest { string user_id = 1; }
message GetUserObligationsResponse { int32 open_buyer_orders = 1; int32 open_seller_orders = 2; int32 open_returns = 3; }

// platform/payment/v1/payment.proto — add to PaymentService. Internal: payment.internal.read; 501 at the edge.
rpc GetPayoutObligations(GetPayoutObligationsRequest) returns (GetPayoutObligationsResponse);
message GetPayoutObligationsRequest { string user_id = 1; }
message GetPayoutObligationsResponse { int64 available_balance = 1; int64 held_balance = 2; int32 pending_payouts = 3; }
// RequestPayout / RequestWalletPayout (comment change): FAILED_PRECONDITION "seller verification required"
// unless the seller's status is VERIFIED; UNAVAILABLE when verification cannot be read.

// platform/verification/v1/verification.proto
VERIFICATION_STATUS_NOT_SUBMITTED = 4;
rpc CreateKycUploadUrl(CreateKycUploadUrlRequest) returns (CreateKycUploadUrlResponse);       // USER principal
rpc ListKycSubmissions(ListKycSubmissionsRequest) returns (ListKycSubmissionsResponse);       // admin
rpc GetKycDocumentUrl(GetKycDocumentUrlRequest) returns (GetKycDocumentUrlResponse);         // admin, audited kyc.document_view first, fail closed
rpc GetPublicBadges(GetPublicBadgesRequest) returns (GetPublicBadgesResponse);               // public (anonymous allowed)
message CreateKycUploadUrlRequest { string content_type = 1; int64 size_bytes = 2; } // image/jpeg|image/png|application/pdf, 1..5 MiB
message CreateKycUploadUrlResponse { string upload_url = 1; string doc_ref = 2; google.protobuf.Timestamp expires_at = 3; }
// SubmitKycRequest: doc_ref must be issued to the caller and uploaded; add
repeated string additional_doc_refs = 3;      // 0..2 more keys, same rules
// ReviewKycRequest: add; required (lower_snake_case ≤ 64) when decision is reject, else empty
string reason_code = 3;
// GetVerificationStatusResponse: add (own or admin view only)
string reject_reason_code = 3;
// GetVerificationStatus: a SERVICE principal with scope verification.read may read any user_id.
message KycSubmission {
  string id = 1; string user_id = 2; string doc_type = 3; VerificationStatus status = 4;
  int32 document_count = 5; string reason_code = 6;
  google.protobuf.Timestamp created_at = 7; google.protobuf.Timestamp reviewed_at = 8;
}
message ListKycSubmissionsRequest { VerificationStatus status = 1; platform.common.v1.PageRequest page = 2; } // oldest first
message ListKycSubmissionsResponse { repeated KycSubmission submissions = 1; platform.common.v1.PageResponse page = 2; }
message GetKycDocumentUrlRequest { string submission_id = 1; int32 index = 2; }   // 0 = doc_ref
message GetKycDocumentUrlResponse { string url = 1; google.protobuf.Timestamp expires_at = 2; }
message GetPublicBadgesRequest { repeated string user_ids = 1; }     // 1..50, else INVALID_ARGUMENT
message PublicBadge { string user_id = 1; bool verified = 2; }
message GetPublicBadgesResponse { repeated PublicBadge badges = 1; }

// platform/audit/v1/audit.proto — action catalogue (25 → 26): add kyc.document_view
```
Existing contracts consumed unchanged: `AuthService.Register`, `AuthService.Login`,
`VerificationService.SubmitKyc/ReviewKyc/GetVerificationStatus`, `AuditService.WriteAuditEvent`,
`ListingService.DeleteListing` path / `listing.events/ListingChanged`, `PaymentService.RequestPayout/RequestWalletPayout`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contracts | trust-legal-contract | platform-core | – | provides: PrivacyService.*, UserErased, OrderService.GetUserObligations, PaymentService.GetPayoutObligations, VerificationService.{CreateKycUploadUrl,ListKycSubmissions,GetKycDocumentUrl,GetPublicBadges} | platform-core |
| legal acceptance at sign-up + consent ledger | identity-legal-consent | team-identity | contract | provides impl: PrivacyService.RecordConsent/ListMyConsents | team-identity, team-gateway |
| user obligations for deletion | commerce-user-erasure | team-order | contract | provides impl: GetUserObligations, GetPayoutObligations; uses: identity.events/UserErased | team-order, team-payment, team-promotion, team-referral |
| account deletion lifecycle | identity-account-deletion | team-identity | identity-legal-consent, commerce-user-erasure | uses: GetUserObligations, GetPayoutObligations; provides impl: identity.events/UserErased | team-identity, team-gateway |
| data export | identity-data-export | team-identity | identity-account-deletion | provides impl: PrivacyService.ExportMyData | team-identity, team-gateway |
| erasure of user content | content-user-erasure | team-engagement | contract | uses: identity.events/UserErased | team-engagement, team-chat, team-notification |
| erasure of erased seller's listings | domain-seller-erasure | team-domain | contract | uses: identity.events/UserErased, listing.events/ListingChanged | team-domain |
| erasure in analytics/ML stores | analytics-user-erasure | team-analytics | contract | uses: identity.events/UserErased | team-analytics, platform-recsys, platform-featurestore, team-ai |
| KYC documents, queue, badges | verification-kyc-documents | team-verification | contract | provides impl: CreateKycUploadUrl, ListKycSubmissions, GetKycDocumentUrl, GetPublicBadges; uses: AuditService.WriteAuditEvent, identity.events/UserErased | team-verification, team-gateway |
| payouts require VERIFIED | payment-payout-kyc-gate | team-payment | verification-kyc-documents | uses: VerificationService.GetVerificationStatus | team-payment, platform-core (seed) |
| legal pages, footer, sign-up acceptance, VN consent banner | frontend-legal-pages | team-frontend | identity-legal-consent | uses: AuthService.Register, PrivacyService.RecordConsent | team-frontend, team-frontend-seller |
| account privacy page (consents, export, delete) | frontend-account-privacy | team-frontend | identity-data-export, frontend-legal-pages | uses: PrivacyService.* | team-frontend |
| seller KYC upload + shop badge | seller-kyc-ui | team-frontend-seller | verification-kyc-documents, payment-payout-kyc-gate | uses: CreateKycUploadUrl, SubmitKyc, GetVerificationStatus, GetPublicBadges | team-frontend-seller, team-frontend |
| admin KYC queue | admin-kyc-queue | team-frontend-admin | verification-kyc-documents | uses: ListKycSubmissions, GetKycDocumentUrl, ReviewKyc | team-frontend-admin |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-gateway/internal/edge/auth.go` (+ PrivacyService forwarder) — identity-legal-consent, identity-account-deletion, identity-data-export
- `team-gateway/internal/edge/policy.go`, `verification.go` — verification-kyc-documents only; `order.go`/`payment.go` 501 entries — commerce-user-erasure
- `docker-compose.services.yaml` (topic init line 26 DLQs, consumer env, MinIO bucket) — every erasure slice, verification-kyc-documents
- `platform-core/tools/seed-marketplace.sh` and platform-e2e payout steps — payment-payout-kyc-gate, seller-kyc-ui
- `team-frontend/src/components/shell/ConsumerShell.tsx` (footer, account nav) — frontend-legal-pages, frontend-account-privacy, seller-kyc-ui (removes buyer KYC link)

## Out of initiative
- Q2/privacy-audit-hardening DPO sign-offs (lawful basis of `hidden_by`/`hide_reason`, session retention) — human items, tracked in the run notes.
- GA4 DPO and cross-border dossier — GA4 initiative follow-up.

## NFR and data
- NFR: erasure of one user completes in every consumer within 1 h of `UserErased` locally (DLQ empty);
  `RecordConsent` p95 ≤ 50 ms; `ExportMyData` p95 ≤ 500 ms, ≤ 1 MiB; payout gate adds ≤ 2 s worst case
  (timeout → `UNAVAILABLE`); presigned URLs TTL 15 min (PUT) / 5 min (GET).
- Personal data:
  - consent records (purpose, version, granted, time, source) — proof of consent (Decree 13 Art. 9-11);
    retained for the account's life and kept after erasure for N years, N set by legal (NEEDS_HUMAN).
  - deletion request timestamps — data-subject request handling; deleted with the erasure, the outcome stays as `USER_STATUS_DELETED`.
  - export file — generated on demand, never stored server-side, `Cache-Control: no-store`.
  - KYC images (ID document with face photo) — seller identification (Decree 85/2021 platform duty); treated
    as sensitive: explicit consent checkbox at upload, private encrypted bucket, admin-only audited access,
    never logged, never in URLs beyond the signed key. Retention: VERIFIED documents are kept while the seller account exists; REJECTED or superseded documents are deleted after 90 days.
  - pseudonymised orders/ledger after erasure — accounting duty; retention per the accounting period set by legal (see the team-order / team-payment rule above).
  - Which law governs: design to the stricter of Decree 13/2023 and the 2026 Personal Data Protection Law; both citations and the deadlines (including the "72 h") are unverified (NEEDS_HUMAN).
- Legacy users without recorded Terms acceptance get the re-acceptance prompt on next sign-in (story 1).
- Existing sellers with no KYC are blocked from payouts at rollout; demo sellers are verified by the seed
  through the real RPCs; there is no grace period locally.
- Risk routing expected: `proto`, `pii`, `auth`, `payment`.
- NEEDS_HUMAN (legal/DPO):
  - Confirm the applicable instruments (Decree 13/2023, 2026 PDP law) and the data-subject-request deadline; the "72 h" and Art. 16 citations are unverified.
  - Supply the text of all seven legal pages and the operator details (company name, registration number, address, MoIT notice).
  - Set the accounting retention period for pseudonymised orders/ledger and N years for consent records after erasure.
  - DPO sign-off on keeping immutable team-audit copies (legal obligation) and on KYC retention (90 days for rejected/superseded).

## Conventions
- Mock data, not mock services: KYC demo state comes from seeding real RPCs; no fake containers, no route fulfils.
- No `*_ENABLED` flags and no in-memory fallbacks: MinIO, Kafka and Postgres are required where used; boot fails without them.
- Never log raw user ids, document keys or consent payloads (go-sdk sloglint forbidden keys); log counts and hashed ids only.
- Images run non-root; the new bucket init uses the existing MinIO init job pattern.
- Admin mutations audit first, fail closed (AGENTS.md §3.6): `GetKycDocumentUrl` → `kyc.document_view`;
  `ReviewKyc` keeps `kyc.review` and adds `reason_code` to metadata.
- Service-to-service calls use the service principal pattern of `service-team-verification` + `audit.write`.
- Every erasure consumer: own consumer group, `identity.events.<svc>.dlq`, idempotent by user id, mirror
  `team-domain/internal/events/identity.go` (UserStatusChanged consumer).
- Tests: Go Docker-backed tests need `GO_SDK_REQUIRE_DOCKER=1`; gates through `scripts/asdlc/gate_digest.py`.
- UI copy is Vietnamese; legal bodies stay placeholders until the legal owner provides text.

## Locked
2026-11-13 — Confirmed by Bùi Đăng Phúc (all defaults accepted). Key decisions: consent records for signed-in users only; deletion blocked by open obligations, requires password, 48 h cancel window and erasure within 72 h; KYC gates payouts only, seller app only; keep reviews/chat text anonymised, keep pseudonymised orders/ledger and audit copies; seven placeholder legal pages; stricter of Decree 13/2023 and 2026 PDP law; no payout grace period.
