## ADDED Requirements

### Requirement: Presigned upload of KYC documents

`VerificationService.CreateKycUploadUrl` SHALL, for a USER principal, return a presigned PUT URL valid 15 minutes for the private
`kyc-documents` bucket, with the content type and length bound in the signature, a `doc_ref` of the form `kyc/<caller id>/<uuid>`,
and SHALL record the grant for the caller. It SHALL accept only `image/jpeg`, `image/png` and `application/pdf` with `size_bytes`
from 1 to 5 MiB, and SHALL allow at most 10 open grants per user.

#### Scenario: Seller receives an upload URL
- **WHEN** a signed-in seller calls `CreateKycUploadUrl` with `image/jpeg` and `size_bytes` 1048576
- **THEN** the response has a `doc_ref` matching `kyc/<own id>/<uuid>`, an `expires_at` 15 minutes (+-30 s) ahead, and a PUT of exactly 1048576 bytes of `image/jpeg` to `upload_url` returns 2xx
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Seller receives an upload URL

#### Scenario: Upload that differs from the signature is refused by storage
- **WHEN** the seller PUTs a body of a different content type or length than the one requested to `upload_url`
- **THEN** the storage endpoint refuses the request with 403 and no object exists under `doc_ref`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Upload that differs from the signature is refused by storage

#### Scenario: Invalid type or size is rejected
- **WHEN** a seller calls `CreateKycUploadUrl` with `image/gif`, or with `size_bytes` 0, or with `size_bytes` 5242881
- **THEN** each call fails `INVALID_ARGUMENT` and no grant row is created
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Invalid type or size is rejected

#### Scenario: Maximum size is accepted
- **WHEN** a seller calls `CreateKycUploadUrl` with `application/pdf` and `size_bytes` 5242880
- **THEN** the call succeeds and returns a `doc_ref`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Maximum size is accepted

#### Scenario: Open grants are capped
- **WHEN** a seller who holds 10 unused unexpired grants calls `CreateKycUploadUrl` again
- **THEN** the call fails `RESOURCE_EXHAUSTED` and the 11th grant does not exist
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Open grants are capped

#### Scenario: Anonymous and service callers cannot request an upload URL
- **WHEN** an anonymous caller, or a service principal, calls `CreateKycUploadUrl`
- **THEN** the anonymous call fails `UNAUTHENTICATED` and the service call fails `PERMISSION_DENIED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Anonymous and service callers cannot request an upload URL

### Requirement: Submission requires uploaded documents issued to the caller

`SubmitKyc` SHALL require `doc_ref` and accept 0..2 `additional_doc_refs`; every key SHALL have been issued to the caller, be
unused, unexpired and uploaded with the granted size and content type, and keys SHALL be distinct. On success it SHALL create one
`PENDING` submission with `1 + len(additional_doc_refs)` documents and mark the grants used; on any failure it SHALL create nothing.

#### Scenario: Submit with one uploaded document
- **WHEN** a seller uploads one image to a granted `doc_ref` and calls `SubmitKyc` with `doc_type` `national_id` and that `doc_ref`
- **THEN** the call returns status `PENDING` and `GetVerificationStatus` for the seller returns `PENDING`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Submit with one uploaded document

#### Scenario: Submit with three documents
- **WHEN** a seller uploads three granted objects and calls `SubmitKyc` with one `doc_ref` and two `additional_doc_refs`
- **THEN** the submission lists `document_count` 3 in `ListKycSubmissions`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Submit with three documents

#### Scenario: More than three documents are rejected
- **WHEN** a seller calls `SubmitKyc` with a `doc_ref` and 3 `additional_doc_refs`
- **THEN** the call fails `INVALID_ARGUMENT` and no submission exists
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::More than three documents are rejected

#### Scenario: Key issued to another user is rejected
- **WHEN** seller B calls `SubmitKyc` with a `doc_ref` that was issued to seller A
- **THEN** the call fails `INVALID_ARGUMENT` with the same message as for an unknown key and no submission exists for seller B
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Key issued to another user is rejected

#### Scenario: Free-text doc_ref is rejected
- **WHEN** a seller calls `SubmitKyc` with `doc_ref` `my-id-card.jpg`
- **THEN** the call fails `INVALID_ARGUMENT` and no submission exists
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Free-text doc_ref is rejected

#### Scenario: Granted but not uploaded key is rejected
- **WHEN** a seller calls `SubmitKyc` with a granted `doc_ref` to which no object was uploaded
- **THEN** the call fails `FAILED_PRECONDITION` and the grant stays unused
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Granted but not uploaded key is rejected

#### Scenario: Oversize or mismatching object is rejected and removed
- **WHEN** a seller calls `SubmitKyc` for a key whose stored object size differs from the granted size
- **THEN** the call fails `FAILED_PRECONDITION` and the object no longer exists in the bucket
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Oversize or mismatching object is rejected and removed

#### Scenario: A key cannot be submitted twice
- **WHEN** a seller calls `SubmitKyc` with a `doc_ref` already used by an earlier submission, or lists the same key twice
- **THEN** the call fails `INVALID_ARGUMENT` and the number of submissions is unchanged
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::A key cannot be submitted twice

#### Scenario: Concurrent submits cannot share a key
- **WHEN** two `SubmitKyc` calls with the same granted key are made concurrently
- **THEN** exactly 1 call succeeds and the other fails `INVALID_ARGUMENT`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Concurrent submits cannot share a key

#### Scenario: Expired grant is rejected
- **WHEN** a seller calls `SubmitKyc` with a key whose grant is older than 15 minutes
- **THEN** the call fails `FAILED_PRECONDITION` and no submission exists
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Expired grant is rejected

#### Scenario: Anonymous caller cannot submit
- **WHEN** an anonymous caller calls `SubmitKyc`
- **THEN** the call fails `UNAUTHENTICATED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Anonymous caller cannot submit

### Requirement: Verification status reflects submissions

`GetVerificationStatus` SHALL return `NOT_SUBMITTED` with `badge=false` for a user with no submission, otherwise the status of the
latest submission; it SHALL return `reject_reason_code` only to the owner or an admin; it SHALL allow a SERVICE principal with scope
`verification.read` to read any `user_id`. `ReviewKyc` SHALL require `reason_code` (lower_snake_case, at most 64 characters) when
rejecting and an empty `reason_code` when approving, and SHALL record it in the `kyc.review` audit metadata.

#### Scenario: Seller who never submitted reads NOT_SUBMITTED
- **WHEN** a signed-in seller with no submission calls `GetVerificationStatus` with an empty `user_id`
- **THEN** the response has status `NOT_SUBMITTED`, `badge` false and an empty `reject_reason_code`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Seller who never submitted reads NOT_SUBMITTED

#### Scenario: Rejected seller sees the reason code
- **WHEN** an admin rejects a pending submission with `reason_code` `document_unreadable` and the seller calls `GetVerificationStatus`
- **THEN** the response has status `REJECTED` and `reject_reason_code` `document_unreadable`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Rejected seller sees the reason code

#### Scenario: Reject without a reason code is rejected
- **WHEN** an admin calls `ReviewKyc` with decision `reject` and an empty `reason_code`, or a `reason_code` `Bad Reason!` or one of 65 characters
- **THEN** each call fails `INVALID_ARGUMENT` and the submission stays `PENDING`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Reject without a reason code is rejected

#### Scenario: Approve with a reason code is rejected
- **WHEN** an admin calls `ReviewKyc` with decision `approve` and a non-empty `reason_code`
- **THEN** the call fails `INVALID_ARGUMENT` and the submission stays `PENDING`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Approve with a reason code is rejected

#### Scenario: Reason code is audited
- **WHEN** an admin rejects a submission with `reason_code` `document_expired`
- **THEN** team-audit holds one `kyc.review` event for that submission whose metadata `reason_code` is `document_expired`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Reason code is audited

#### Scenario: Another user cannot read a status
- **WHEN** seller A calls `GetVerificationStatus` with the `user_id` of seller B
- **THEN** the call fails `PERMISSION_DENIED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Another user cannot read a status

#### Scenario: Payment service reads any seller's status
- **WHEN** a service principal with scope `verification.read` calls `GetVerificationStatus` for a seller's `user_id`
- **THEN** the response carries that seller's status and an empty `reject_reason_code`
- **VERIFIED BY**: team-verification `internal/rpc` `TestGetVerificationStatusServiceWithScopeReadsAnyUser`

#### Scenario: Service principal without the scope is refused
- **WHEN** a service principal lacking `verification.read` calls `GetVerificationStatus` for another user
- **THEN** the call fails `PERMISSION_DENIED`
- **VERIFIED BY**: team-verification `internal/rpc` `TestGetVerificationStatusServiceWithoutScopeDenied`

#### Scenario: Reviewer cannot review their own submission
- **WHEN** an admin who submitted KYC calls `ReviewKyc` on their own submission
- **THEN** the call fails `PERMISSION_DENIED` and the submission stays `PENDING`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Reviewer cannot review their own submission

### Requirement: Admin queue lists submissions oldest first

`ListKycSubmissions` SHALL require the `admin` scope, return submissions ordered by `created_at` ascending then id, filter on `status`
when set (`UNSPECIFIED` returns all), and page with default 20 and maximum 100. It SHALL NOT return document keys.

#### Scenario: Pending queue is paged oldest first
- **WHEN** an admin lists status `PENDING` with `page_size` 10 over 25 pending submissions, following `next_cursor`
- **THEN** pages hold 10, 10 and 5 submissions in ascending `created_at` order with no duplicates, and the last page has an empty `next_cursor`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Pending queue is paged oldest first

#### Scenario: Page size is clamped and bad cursor rejected
- **WHEN** an admin lists with `page_size` 500, and separately with cursor `garbage`
- **THEN** the first call returns at most 100 submissions and the second fails `INVALID_ARGUMENT`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Page size is clamped and bad cursor rejected

#### Scenario: Filtering by NOT_SUBMITTED is rejected
- **WHEN** an admin lists with status `NOT_SUBMITTED`
- **THEN** the call fails `INVALID_ARGUMENT`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Filtering by NOT_SUBMITTED is rejected

#### Scenario: Non-admin cannot list
- **WHEN** a seller without the `admin` scope, or an anonymous caller, calls `ListKycSubmissions`
- **THEN** the seller call fails `PERMISSION_DENIED` and the anonymous call fails `UNAUTHENTICATED`, at the gateway and at the service
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Non-admin cannot list

### Requirement: Admin document access is audited and fail closed

`GetKycDocumentUrl` SHALL require the `admin` scope, write a `kyc.document_view` audit event (metadata submission id and index, never the key)
before presigning, and return a presigned GET valid 5 minutes only when the audit write succeeded. `index` 0 is `doc_ref`.

#### Scenario: Admin opens a document
- **WHEN** an admin calls `GetKycDocumentUrl` with a pending submission id and index 0
- **THEN** the response `expires_at` is 5 minutes (+-30 s) ahead, a GET of `url` returns the uploaded bytes, and team-audit holds exactly one `kyc.document_view` event by that admin for that submission
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Admin opens a document

#### Scenario: Audit outage blocks the document
- **WHEN** team-audit is unavailable and an admin calls `GetKycDocumentUrl`
- **THEN** the call fails `UNAVAILABLE` and the response contains no `url`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Audit outage blocks the document

#### Scenario: Index out of range or unknown submission
- **WHEN** an admin calls `GetKycDocumentUrl` with index 3 on a submission with 3 documents, or with an unknown submission id
- **THEN** the first call fails `INVALID_ARGUMENT` and the second fails `NOT_FOUND`, and neither writes an audit event
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Index out of range or unknown submission

#### Scenario: Expired link stops working
- **WHEN** a presigned GET is used more than 5 minutes after issue
- **THEN** the storage endpoint refuses it with 403
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Expired link stops working

#### Scenario: Non-admin cannot obtain a document URL
- **WHEN** the submission owner, another seller, or an anonymous caller calls `GetKycDocumentUrl`
- **THEN** the seller calls fail `PERMISSION_DENIED`, the anonymous call fails `UNAUTHENTICATED`, and no `kyc.document_view` event is written
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Non-admin cannot obtain a document URL

#### Scenario: Bucket is not publicly readable
- **WHEN** an anonymous client requests an object in `kyc-documents` without a signature
- **THEN** the storage endpoint refuses the request with 403 and the bucket is configured with default server-side encryption
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Bucket is not publicly readable

### Requirement: Public badge exposes only a boolean

`GetPublicBadges` SHALL accept 1..50 user ids from any caller including anonymous, and SHALL return one entry per distinct id with
`verified` true only when that user's latest submission is `VERIFIED`; it SHALL NOT expose any other status, reason or timestamp.

#### Scenario: Verified seller shows a badge
- **WHEN** an anonymous caller calls `GetPublicBadges` with a verified seller, a pending seller, a rejected seller and an unknown id
- **THEN** the response has 4 entries with `verified` true only for the verified seller
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Verified seller shows a badge

#### Scenario: Empty or oversized id list is rejected
- **WHEN** a caller calls `GetPublicBadges` with 0 ids, or with 51 ids
- **THEN** each call fails `INVALID_ARGUMENT`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Empty or oversized id list is rejected

#### Scenario: Boundary of 50 ids is accepted
- **WHEN** a caller calls `GetPublicBadges` with 50 distinct ids
- **THEN** the call succeeds with 50 entries
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Boundary of 50 ids is accepted

### Requirement: Retention and erasure of KYC documents

team-verification SHALL delete the stored documents of a REJECTED submission 90 days after its rejection and of a superseded
submission 90 days after its supersession, keep VERIFIED unsuperseded documents while the account exists, and on
`identity.events/UserErased` SHALL delete all objects and rows of that user idempotently, with own consumer group and DLQ
`identity.events.team-verification.dlq`.

#### Scenario: Rejected documents are deleted after 90 days
- **WHEN** the sweeper runs on a submission rejected 91 days ago
- **THEN** its objects no longer exist in the bucket, `GetKycDocumentUrl` for it fails `NOT_FOUND`, and its status remains `REJECTED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Rejected documents are deleted after 90 days

#### Scenario: Documents inside the window are kept
- **WHEN** the sweeper runs on a submission rejected 89 days ago, and on a verified unsuperseded submission
- **THEN** all their objects still exist
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Documents inside the window are kept

#### Scenario: Superseded documents are deleted after 90 days
- **WHEN** a seller submitted again 91 days ago and the sweeper runs on the earlier submission
- **THEN** the earlier submission's objects no longer exist and the newer submission's objects still exist
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Superseded documents are deleted after 90 days

#### Scenario: Object delete failure is retried
- **WHEN** the sweeper cannot delete an object from storage
- **THEN** the row is not marked deleted and the next run deletes it
- **VERIFIED BY**: team-verification `internal/retention` `TestSweeperRetriesFailedObjectDelete`

#### Scenario: UserErased removes the user's KYC data
- **WHEN** `UserErased` for a seller with a verified and a rejected submission is consumed
- **THEN** within 1 hour no object under `kyc/<user>/` exists, `ListKycSubmissions` holds no row of that user, and `GetVerificationStatus` returns `NOT_SUBMITTED`
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::UserErased removes the user's KYC data

#### Scenario: Duplicate UserErased is harmless
- **WHEN** the same `UserErased` event is delivered twice
- **THEN** the second delivery succeeds without error and the DLQ `identity.events.team-verification.dlq` has 0 messages
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Duplicate UserErased is harmless

#### Scenario: Erasure of one user leaves others untouched
- **WHEN** `UserErased` for seller A is consumed
- **THEN** seller B's submissions and objects are unchanged
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Erasure of one user leaves others untouched

#### Scenario: Storage outage sends the event to retry then DLQ
- **WHEN** MinIO is unavailable while `UserErased` is consumed and stays unavailable past the retry limit
- **THEN** the event lands on `identity.events.team-verification.dlq` and no database row of that user was deleted before the object delete succeeded
- **VERIFIED BY**: team-verification `internal/events` `TestUserErasedStorageOutageRetriesThenDLQ`

### Requirement: Gateway exposes the new RPCs with the right policy

team-gateway SHALL forward `CreateKycUploadUrl`, `ListKycSubmissions`, `GetKycDocumentUrl` and `GetPublicBadges` to team-verification,
SHALL require the `admin` scope at the edge for `ListKycSubmissions` and `GetKycDocumentUrl`, and SHALL allow `GetPublicBadges` without a token.

#### Scenario: Anonymous public badge call succeeds through the gateway
- **WHEN** a client without a token calls `GetPublicBadges` through the gateway with one id
- **THEN** the response status is 200 and the body holds one entry
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Anonymous public badge call succeeds through the gateway

#### Scenario: Admin-only RPCs refuse non-admins at the edge
- **WHEN** a seller token calls `ListKycSubmissions` through the gateway
- **THEN** the gateway returns 403 and team-verification receives no call
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Admin-only RPCs refuse non-admins at the edge

#### Scenario: Unauthenticated upload URL request is refused at the edge
- **WHEN** a client without a token calls `CreateKycUploadUrl` through the gateway
- **THEN** the gateway returns 401
- **VERIFIED BY**: platform-e2e/tests/e2e/features/verification/kyc_documents.feature::Unauthenticated upload URL request is refused at the edge
