Feature: KYC documents are uploaded, reviewed, listed and erased through private storage
  Scenario titles are the spec scenarios of openspec/changes/verification-kyc-documents
  (capability kyc-documents). Sellers upload to presigned URLs for the private kyc-documents bucket,
  SubmitKyc accepts only keys issued to the caller, admins review through an oldest-first queue and an
  audited document link, and retention and UserErased remove the stored files.

  # ── Presigned upload ────────────────────────────────────────────────────

  Scenario: Seller receives an upload URL
    Given seller "A" is registered
    When seller "A" requests an upload URL for "image/jpeg" of 1048576 bytes
    Then the call succeeds
    And the doc_ref matches "kyc/<A id>/<uuid>"
    And the upload expires in 15 minutes
    And seller "A" uploading 1048576 bytes of "image/jpeg" to the upload URL returns 2xx

  Scenario: Upload that differs from the signature is refused by storage
    Given seller "A" is registered
    When seller "A" requests an upload URL for "image/jpeg" of 1024 bytes
    And seller "A" uploads 2048 bytes of "image/jpeg" to the upload URL
    Then the storage endpoint answers 403
    And no object exists under the doc_ref
    When seller "A" requests an upload URL for "image/jpeg" of 1024 bytes
    And seller "A" uploads 1024 bytes of "image/png" to the upload URL
    Then the storage endpoint answers 403
    And no object exists under the doc_ref

  Scenario: Invalid type or size is rejected
    Given seller "A" is registered
    When seller "A" requests an upload URL for "image/gif" of 1024 bytes
    Then the call fails with code "invalid_argument"
    When seller "A" requests an upload URL for "image/jpeg" of 0 bytes
    Then the call fails with code "invalid_argument"
    When seller "A" requests an upload URL for "image/jpeg" of 5242881 bytes
    Then the call fails with code "invalid_argument"
    And seller "A" has 0 upload grants

  Scenario: Maximum size is accepted
    Given seller "A" is registered
    When seller "A" requests an upload URL for "application/pdf" of 5242880 bytes
    Then the call succeeds
    And the doc_ref matches "kyc/<A id>/<uuid>"

  Scenario: Open grants are capped
    Given seller "A" is registered
    And seller "A" holds 10 unused unexpired upload grants
    When seller "A" requests an upload URL for "image/jpeg" of 1024 bytes
    Then the call fails with code "resource_exhausted"
    And seller "A" has 10 upload grants

  Scenario: Anonymous and service callers cannot request an upload URL
    When an anonymous caller requests an upload URL for "image/jpeg" of 1024 bytes
    Then the call fails with code "unauthenticated"

  # ── Submission ──────────────────────────────────────────────────────────

  Scenario: Submit with one uploaded document
    Given seller "A" is registered
    And seller "A" has uploaded 1 documents
    When seller "A" submits KYC with the uploaded documents
    Then the call succeeds
    And the submission status is "VERIFICATION_STATUS_PENDING"
    And seller "A" reads verification status "VERIFICATION_STATUS_PENDING"

  Scenario: Submit with three documents
    Given seller "A" is registered
    And seller "A" has uploaded 3 documents
    When seller "A" submits KYC with the uploaded documents
    Then the call succeeds
    And the admin queue lists seller "A" submission with document_count 3

  Scenario: More than three documents are rejected
    Given seller "A" is registered
    And seller "A" has uploaded 4 documents
    When seller "A" submits KYC with the uploaded documents
    Then the call fails with code "invalid_argument"
    And seller "A" has 0 submissions

  Scenario: Key issued to another user is rejected
    Given seller "A" is registered
    And seller "B" is registered
    And seller "A" has uploaded 1 documents
    When seller "B" submits KYC with the documents of seller "A"
    Then the call fails with code "invalid_argument"
    And the error message equals the one for an unknown key
    And seller "B" has 0 submissions

  Scenario: Free-text doc_ref is rejected
    Given seller "A" is registered
    When seller "A" submits KYC with the free-text doc_ref "my-id-card.jpg"
    Then the call fails with code "invalid_argument"
    And seller "A" has 0 submissions

  Scenario: Granted but not uploaded key is rejected
    Given seller "A" is registered
    And seller "A" holds 1 unused unexpired upload grants
    When seller "A" submits KYC with the granted keys
    Then the call fails with code "failed_precondition"
    And seller "A" has 0 used upload grants

  Scenario: Oversize or mismatching object is rejected and removed
    Given seller "A" is registered
    And seller "A" holds 1 unused unexpired upload grants
    And an object of 2097152 bytes is stored directly under the granted key
    When seller "A" submits KYC with the granted keys
    Then the call fails with code "failed_precondition"
    And no object exists under the granted key

  Scenario: A key cannot be submitted twice
    Given seller "A" is registered
    And seller "A" has uploaded 1 documents
    And seller "A" has submitted KYC with the uploaded documents
    When seller "A" submits KYC with the uploaded documents
    Then the call fails with code "invalid_argument"
    And seller "A" has 1 submissions
    Given seller "A" has uploaded 1 documents
    When seller "A" submits KYC listing the first key twice
    Then the call fails with code "invalid_argument"
    And seller "A" has 1 submissions

  Scenario: Concurrent submits cannot share a key
    Given seller "A" is registered
    And seller "A" has uploaded 1 documents
    When seller "A" submits KYC with the uploaded documents twice concurrently
    Then exactly 1 call succeeds and the other fails with code "invalid_argument"
    And seller "A" has 1 submissions

  Scenario: Expired grant is rejected
    Given seller "A" is registered
    And seller "A" has uploaded 1 documents
    And the grants of seller "A" were issued 16 minutes ago
    When seller "A" submits KYC with the uploaded documents
    Then the call fails with code "failed_precondition"
    And seller "A" has 0 submissions

  Scenario: Anonymous caller cannot submit
    When an anonymous caller submits KYC with the free-text doc_ref "kyc/nobody/00000000-0000-0000-0000-000000000000"
    Then the call fails with code "unauthenticated"

  # ── Status and review ───────────────────────────────────────────────────

  Scenario: Seller who never submitted reads NOT_SUBMITTED
    Given seller "A" is registered
    When seller "A" reads verification status "VERIFICATION_STATUS_NOT_SUBMITTED" with an empty user_id
    Then the status response has badge false and an empty reject_reason_code

  Scenario: Rejected seller sees the reason code
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And an admin is logged in
    When the admin rejects the submission of seller "A" with reason_code "document_unreadable"
    Then the call succeeds
    And seller "A" reads verification status "VERIFICATION_STATUS_REJECTED"
    And the reject_reason_code is "document_unreadable"

  Scenario: Reject without a reason code is rejected
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And an admin is logged in
    When the admin rejects the submission of seller "A" with reason_code ""
    Then the call fails with code "invalid_argument"
    When the admin rejects the submission of seller "A" with reason_code "Bad Reason!"
    Then the call fails with code "invalid_argument"
    When the admin rejects the submission of seller "A" with a reason_code of 65 characters
    Then the call fails with code "invalid_argument"
    And seller "A" reads verification status "VERIFICATION_STATUS_PENDING"

  Scenario: Approve with a reason code is rejected
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And an admin is logged in
    When the admin approves the submission of seller "A" with reason_code "looks_fine"
    Then the call fails with code "invalid_argument"
    And seller "A" reads verification status "VERIFICATION_STATUS_PENDING"

  Scenario: Reason code is audited
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And an admin is logged in
    When the admin rejects the submission of seller "A" with reason_code "document_expired"
    Then the call succeeds
    And exactly one kyc.review audit event exists for the submission of seller "A" with reason_code "document_expired"

  Scenario: Another user cannot read a status
    Given seller "A" is registered
    And seller "B" is registered
    When seller "A" reads the verification status of seller "B"
    Then the call fails with code "permission_denied"

  Scenario: Reviewer cannot review their own submission
    Given an admin is logged in
    And the admin has a pending KYC submission
    When the admin approves their own submission
    Then the call fails with code "permission_denied"
    And the admin reads verification status "VERIFICATION_STATUS_PENDING"

  # ── Admin queue ─────────────────────────────────────────────────────────

  Scenario: Pending queue is paged oldest first
    Given 25 sellers each have a pending KYC submission
    And an admin is logged in
    When the admin walks the PENDING queue with page_size 10 following next_cursor
    Then every page but the last holds 10 submissions in ascending created_at order without duplicates
    And the last page has an empty next_cursor
    And the 25 submissions appear in submission order at the end of the queue
    And no listed submission carries a document key

  Scenario: Page size is clamped and bad cursor rejected
    Given an admin is logged in
    When the admin lists the queue with page_size 500
    Then the call succeeds
    And at most 100 submissions are returned
    When the admin lists the queue with cursor "garbage"
    Then the call fails with code "invalid_argument"

  Scenario: Filtering by NOT_SUBMITTED is rejected
    Given an admin is logged in
    When the admin lists the queue with status "VERIFICATION_STATUS_NOT_SUBMITTED"
    Then the call fails with code "invalid_argument"

  Scenario: Non-admin cannot list
    Given seller "A" is registered
    When seller "A" lists the queue
    Then the call fails with code "permission_denied"
    When an anonymous caller lists the queue
    Then the call fails with code "unauthenticated"

  # ── Document access ─────────────────────────────────────────────────────

  Scenario: Admin opens a document
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And an admin is logged in
    When the admin requests document 0 of the submission of seller "A"
    Then the call succeeds
    And the document link expires in 5 minutes
    And a GET of the document link returns the uploaded bytes
    And exactly one kyc.document_view audit event exists for the submission of seller "A" by the admin

  @destructive
  Scenario: Audit outage blocks the document
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And an admin is logged in
    When team-audit is stopped
    And the admin requests document 0 of the submission of seller "A"
    Then the call fails with code "unavailable"
    And the response contains no url
    When team-audit is started again

  Scenario: Index out of range or unknown submission
    Given seller "A" is registered
    And seller "A" has uploaded 3 documents
    And seller "A" has submitted KYC with the uploaded documents
    And an admin is logged in
    When the admin requests document 3 of the submission of seller "A"
    Then the call fails with code "invalid_argument"
    When the admin requests document 0 of an unknown submission
    Then the call fails with code "not_found"
    And no kyc.document_view audit event exists for the submission of seller "A"

  @slow
  Scenario: Expired link stops working
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And an admin is logged in
    When the admin requests document 0 of the submission of seller "A"
    And more than 5 minutes pass since the document link was issued
    Then a GET of the document link answers 403

  Scenario: Non-admin cannot obtain a document URL
    Given seller "A" is registered
    And seller "B" is registered
    And seller "A" has a pending KYC submission
    When seller "A" requests document 0 of their own submission
    Then the call fails with code "permission_denied"
    When seller "B" requests document 0 of the submission of seller "A"
    Then the call fails with code "permission_denied"
    When an anonymous caller requests document 0 of the submission of seller "A"
    Then the call fails with code "unauthenticated"
    And no kyc.document_view audit event exists for the submission of seller "A"

  Scenario: Bucket is not publicly readable
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    When an anonymous client requests the first object of seller "A" without a signature
    Then the storage endpoint answers 403
    And the kyc-documents bucket has default server-side encryption

  # ── Public badge ────────────────────────────────────────────────────────

  Scenario: Verified seller shows a badge
    Given seller "V" has a verified KYC submission
    And seller "P" has a pending KYC submission
    And seller "R" has a rejected KYC submission with reason_code "document_unreadable"
    When an anonymous caller asks for the public badges of sellers "V", "P", "R" and an unknown id
    Then the call succeeds
    And 4 badge entries are returned with verified true only for seller "V"

  Scenario: Empty or oversized id list is rejected
    When an anonymous caller asks for the public badges of 0 ids
    Then the call fails with code "invalid_argument"
    When an anonymous caller asks for the public badges of 51 ids
    Then the call fails with code "invalid_argument"

  Scenario: Boundary of 50 ids is accepted
    When an anonymous caller asks for the public badges of 50 ids
    Then the call succeeds
    And 50 badge entries are returned

  # ── Retention and erasure ───────────────────────────────────────────────

  @slow
  Scenario: Rejected documents are deleted after 90 days
    Given seller "A" is registered
    And seller "A" has a rejected KYC submission with reason_code "document_unreadable"
    And an admin is logged in
    And the rejection of seller "A" happened 91 days ago
    When the retention sweeper has run
    Then no object of the submission of seller "A" exists
    And the admin requesting document 0 of the submission of seller "A" fails with code "not_found"
    And seller "A" reads verification status "VERIFICATION_STATUS_REJECTED"

  @slow
  Scenario: Documents inside the window are kept
    Given seller "A" has a rejected KYC submission with reason_code "document_unreadable"
    And seller "V" has a verified KYC submission
    And the rejection of seller "A" happened 89 days ago
    And the approval of seller "V" happened 200 days ago
    And a rejected sweep marker submission rejected 91 days ago
    When the retention sweeper has run
    Then all objects of the submission of seller "A" exist
    And all objects of the submission of seller "V" exist

  @slow
  Scenario: Superseded documents are deleted after 90 days
    Given seller "A" is registered
    And seller "A" has a pending KYC submission
    And seller "A" has a second KYC submission
    And the first submission of seller "A" was superseded 91 days ago
    When the retention sweeper has run
    Then no object of the first submission of seller "A" exists
    And all objects of the second submission of seller "A" exist

  @slow
  Scenario: UserErased removes the user's KYC data
    Given seller "A" has a verified KYC submission
    And seller "A" has a rejected second KYC submission with reason_code "document_expired"
    When UserErased for seller "A" is published on identity.events
    Then within the erasure window no object exists under the key prefix of seller "A"
    And the admin queue holds no submission of seller "A"
    And the admin reads the verification status of seller "A" as "VERIFICATION_STATUS_NOT_SUBMITTED"

  @slow
  Scenario: Duplicate UserErased is harmless
    Given seller "A" has a verified KYC submission
    When UserErased for seller "A" is published on identity.events twice with the same event id
    Then within the erasure window no object exists under the key prefix of seller "A"
    And the team-verification DLQ holds no message for seller "A"

  @slow
  Scenario: Erasure of one user leaves others untouched
    Given seller "A" has a verified KYC submission
    And seller "B" has a verified KYC submission
    When UserErased for seller "A" is published on identity.events
    Then within the erasure window no object exists under the key prefix of seller "A"
    And seller "B" still has 1 submissions and all objects of the submission of seller "B" exist

  # ── Gateway policy ──────────────────────────────────────────────────────

  Scenario: Anonymous public badge call succeeds through the gateway
    When an anonymous caller asks for the public badges of 1 ids
    Then the call succeeds
    And 1 badge entries are returned

  Scenario: Admin-only RPCs refuse non-admins at the edge
    Given seller "A" is registered
    When seller "A" lists the queue
    Then the gateway answers 403
    When seller "A" requests document 0 of an unknown submission
    Then the gateway answers 403

  Scenario: Unauthenticated upload URL request is refused at the edge
    When an anonymous caller requests an upload URL for "image/jpeg" of 1024 bytes
    Then the gateway answers 401
