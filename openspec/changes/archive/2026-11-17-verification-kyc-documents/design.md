# Design

## Storage
- Bucket `kyc-documents`, private, default server-side encryption (SSE-S3) set by the bucket init job. The service never proxies
  bytes: `CreateKycUploadUrl` signs a PUT with `Content-Type` and `Content-Length` as signed headers (15 min), key `kyc/<user>/<uuid>`.
- Because a presigned PUT does not by itself cap the body for every S3 implementation, `SubmitKyc` re-checks with a stat call:
  size equals the granted size and is <= 5 MiB, content type equals the granted type. An oversize or mismatching object is deleted
  and the submit is rejected.
- The MinIO client mirrors `team-domain/internal/objects/storage.go` config (endpoint, credentials, SSL flag, bucket) and sits behind an
  interface so unit tests use a fake; integration tests use a real MinIO container (`GO_SDK_REQUIRE_DOCKER=1`).

## Data model (migration `002_kyc_documents`)
- `kyc_upload_grants(object_key pk, user_id, content_type, size_bytes, issued_at, expires_at, used_at null)`; index `(user_id, used_at, expires_at)`.
- `kyc_documents(submission_id fk, idx smallint, object_key, content_type, size_bytes, deleted_at null)`; `idx 0` is `doc_ref`.
  `kyc_submissions.doc_ref` stays for compatibility and equals document 0.
- `kyc_submissions` gains `reason_code varchar(64) null`, `superseded_at timestamptz null`.
- Submit runs in one transaction: validate grants, insert submission, insert documents, mark grants used, set `superseded_at = now()` on the
  user's previous submissions that are still unsuperseded. Grants are checked `FOR UPDATE` so two concurrent submits cannot use one key.
- Status stays "latest submission by created_at", so a new PENDING submission after VERIFIED reads PENDING (existing behaviour, unchanged).

## Retention
- Sweeper (pattern of team-identity `session_sweeper.go`, in-process, hourly): select documents with `deleted_at is null` of submissions that
  are REJECTED with `reviewed_at < now() - 90 days` or have `superseded_at < now() - 90 days`; delete the object first, then set `deleted_at`.
  A failed object delete leaves the row for the next run (idempotent). The submission row stays (status, reason) with `document_count` 0.
- VERIFIED and unsuperseded documents are kept while the account exists.

## Erasure
- Consumer of `identity.events/UserErased` mirrors `team-domain/internal/events/identity.go`: own consumer group, DLQ
  `identity.events.team-verification.dlq`, idempotent on `user_id`. It deletes all objects under `kyc/<user>/`, then rows from
  `kyc_documents`, `kyc_submissions`, `kyc_upload_grants`. Objects first, so a crash leaves rows to retry, never orphan objects.
- This deletes VERIFIED documents too; whether they must be kept for a legal period is an open DPO question (see OPEN_ISSUES in the run).
- team-verification gains `identity.proto` and `events.proto` copies (added by trust-legal-contract) and a Kafka dependency through the
  existing go-sdk kafka package; it previously had no Kafka.

## Audit
- `GetKycDocumentUrl`: validate submission and index, then `auditx.Gate.Do` with action `kyc.document_view`, target `kyc_submission`,
  metadata `{user_id, index}`; the presign runs inside the gate callback, so an audit error returns `UNAVAILABLE` and no URL exists.
- `ReviewKyc` keeps `kyc.review`; metadata adds `reason_code` (empty on approve).

## Errors
- not issued / foreign / malformed / duplicate key: `INVALID_ARGUMENT` (same message for unknown and foreign, no enumeration).
- key issued but not uploaded, or object mismatches the grant: `FAILED_PRECONDITION`.
- expired grant: `FAILED_PRECONDITION`. Grant cap of 10 open (unused, unexpired) grants per user: `RESOURCE_EXHAUSTED`.
- `GetKycDocumentUrl` with a document deleted by retention: `NOT_FOUND`.

## Gateway
- `VerificationForwarder` gets `CreateKycUploadUrl` (write), `ListKycSubmissions` (read), `GetKycDocumentUrl` (read), `GetPublicBadges` (read, anonymous).
  `adminProcedures` gains the two admin RPCs; the pinned-set test is updated. Service principals are not minted at the edge:
  team-payment reaches `GetVerificationStatus` directly on the verification gRPC port.
