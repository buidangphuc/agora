## Why

Today a seller "submits KYC" by typing a free-text `doc_ref` (`team-frontend/src/features/account/verification/SubmitKycForm.tsx:26,33`);
team-verification stores it as-is (`migrations/001_create_kyc_submissions.up.sql`), admins have no way to list submissions or see
a document, a seller who never submitted reads `PENDING`, and no public surface shows who is verified. Backlog Q28 phase 1 needs
real identity documents, an admin work queue, and a boolean badge, before team-payment can gate payouts on `VERIFIED`
(slice payment-payout-kyc-gate).

initiative: trust-legal

## What changes

- team-verification gets a private, encrypted MinIO bucket `kyc-documents` and a MinIO client (same SDK as team-domain
  `internal/objects/storage.go`). New RPC `CreateKycUploadUrl` (USER) returns a presigned PUT (15 min, content type and length
  bound in the signature, key `kyc/<user>/<uuid>`) and records the issued key for the caller.
- `SubmitKyc` accepts `doc_ref` plus `additional_doc_refs` (0..2): every key must have been issued to the caller, be unused and
  be uploaded (object exists, size and content type match what was issued); otherwise the call is rejected and nothing is stored.
- `GetVerificationStatus`: a user with no submission reads `NOT_SUBMITTED` (was `PENDING`); the response carries
  `reject_reason_code` for the owner or an admin; a SERVICE principal with scope `verification.read` may read any `user_id`
  (used by team-payment).
- `ReviewKyc` takes `reason_code` (required on reject, rejected otherwise) and writes it into the `kyc.review` audit metadata.
- New admin RPCs `ListKycSubmissions` (oldest first, paged, optional status filter) and `GetKycDocumentUrl` (writes
  `kyc.document_view` audit first, fail closed, returns a 5-minute presigned GET).
- New public RPC `GetPublicBadges` (1..50 user ids, anonymous allowed) returning only `verified` booleans.
- Retention and erasure: an in-process sweeper deletes documents of REJECTED and superseded submissions 90 days after the
  rejection or supersession; a consumer of `identity.events/UserErased` (own consumer group, DLQ
  `identity.events.team-verification.dlq`, idempotent by user id) deletes the user's submissions and stored documents.
- team-gateway: `VerificationForwarder` forwards the four new RPCs; `policy.go` gates `ListKycSubmissions` and
  `GetKycDocumentUrl` with the `admin` scope; `GetPublicBadges` is open to anonymous callers.
- Compose: `kyc-documents` bucket init in the existing MinIO init job pattern, topic/DLQ init line, service env.

Contract consumed (from slice trust-legal-contract, not redefined here): `CreateKycUploadUrl*`, `ListKycSubmissions*`,
`GetKycDocumentUrl*`, `GetPublicBadges*`, `KycSubmission`, `PublicBadge`, `VERIFICATION_STATUS_NOT_SUBMITTED=4`,
`SubmitKycRequest.additional_doc_refs=3`, `ReviewKycRequest.reason_code=3`, `GetVerificationStatusResponse.reject_reason_code=3`,
audit action `kyc.document_view`, `UserErased`.

## Capabilities

### New Capabilities
- `kyc-documents`: document upload, admin queue and audited document access, status semantics, public badge, retention and erasure.

### Modified Capabilities
none

## Impact

- team-verification: `internal/config` (MinIO, Kafka settings), new `internal/objects` (presign, stat, delete), `internal/store`
  (migration `002_kyc_documents`, queries), `internal/verification`, `internal/rpc/verification.go`, new `internal/events`
  (UserErased consumer) and `internal/retention` (sweeper), `internal/app/app.go`, `Dockerfile` env, `README.md` contract
  table, `FEATURES.yaml` (by e2e-author), vendored protos (`identity.proto`, `events.proto` added by the contract slice).
- team-gateway: `internal/edge/verification.go`, `internal/edge/policy.go` (hot files, only this slice edits them).
- `docker-compose.services.yaml` (shared hot file, serialised by initiative-build).
- Existing callers of `SubmitKyc` with a free-text `doc_ref` stop working: the buyer form is removed by slice seller-kyc-ui; the
  seed (`platform-core/tools/seed-marketplace.sh`, owned by payment-payout-kyc-gate) must use the real upload path.

## Non-goals

Real eKYC vendor, liveness, NFC read, legal-name capture and bank-name matching, gating listings on KYC, the payout gate itself
(slice payment-payout-kyc-gate), the seller upload screen (seller-kyc-ui), the admin tab (admin-kyc-queue), MFA for admins,
virus scanning of uploads, image content inspection.

## Non-functional requirements

- Presigned PUT TTL 15 min; presigned GET TTL 5 min; upload size 1 byte to 5 MiB; types `image/jpeg`, `image/png`, `application/pdf`.
- `GetVerificationStatus` p95 <= 50 ms at 20 rps (team-payment budget for the call is 2 s, single attempt).
- `ListKycSubmissions` p95 <= 200 ms for a 10 000-row table; page size default 20, clamp 100.
- `GetPublicBadges` p95 <= 100 ms for 50 ids; one query, no per-id round trip.
- `GetKycDocumentUrl` adds one audit write bounded to 2 s (`auditx.Gate`); audit failure means no URL (fail closed).
- Erasure of one user completes in team-verification within 1 h of `UserErased` locally, DLQ empty.
- Boot fails without Postgres, MinIO, Kafka or `UPSTREAM_AUDIT_ADDR`; no `*_ENABLED` flags and no in-memory fallback.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| KYC document object (ID image or PDF with face photo) | sensitive personal | seller identification (Decree 85/2021 platform duty, as cited by the initiative, unverified, NEEDS_HUMAN) | explicit consent checkbox at upload (UI in seller-kyc-ui) plus legal obligation | VERIFIED: while the seller account exists; REJECTED or superseded: deleted 90 days after rejection or supersession; all deleted on `UserErased` (see design) |
| `kyc_documents.object_key`, `kyc_upload_grants.object_key` | sensitive personal (locator to the document) | find the object | same | same as the object; never logged, never returned except inside a signed URL |
| `kyc_submissions.user_id`, `doc_type`, `status`, `reason_code`, timestamps | personal | review workflow and payout gate | legal obligation | as the object, then the row is deleted on `UserErased` |
| audit event `kyc.document_view` (actor admin id, submission id, index) | personal (admin id) | accountability for access to sensitive data (ASVS V8.3) | legitimate interest, legal obligation | immutable in team-audit, pending DPO sign-off (NEEDS_HUMAN) |
| public badge boolean | derived, non-identifying beyond the shop id | trust signal | legitimate interest | not stored separately |

Document keys, user ids, signed URLs and file bytes are never logged (go-sdk sloglint forbidden keys); logs carry counts, the
submission hash and the outcome only. Images never stream through any service; only presigned URLs are issued.

## Threat model

- Assets: ID document images, the verified flag (it unlocks money), the integrity of the review decision, the audit trail.
- Abuse cases: (1) Submitting another user's key or a guessable key: keys are random uuids, must match a grant row of the caller,
  and the rejection is the same for unknown and foreign keys. (2) Submitting a key that was never uploaded or whose size or type
  differs from the grant: object stat check, rejected. (3) Reusing one verified document across accounts: a key is single-use
  and bound to one grant. (4) Upload flooding / storage abuse: size and type bound in the signature, at most 10 open
  grants per user. (5) Admin browsing documents without trace: URL only through `GetKycDocumentUrl`, audit first, fail closed.
  (6) Non-admin reading documents or the queue: `admin` scope required, edge and service both check. (7) Enumerating who is verified
  or why someone was rejected publicly: `GetPublicBadges` returns booleans only; `reject_reason_code` only to owner or admin.
  (8) Forging `VERIFIED`: only `ReviewKyc` (admin, not own submission) writes it. (9) Reading status of any user: needs admin or a
  service principal with `verification.read`.
- Authz rules: CreateKycUploadUrl and SubmitKyc USER principal, owner = principal id; ListKycSubmissions, GetKycDocumentUrl,
  ReviewKyc `admin` scope; GetVerificationStatus own/admin/service(`verification.read`); GetPublicBadges anyone.
