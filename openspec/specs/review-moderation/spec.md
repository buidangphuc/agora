# review-moderation Specification

## Purpose
Admins hide a review with an audited reason, which removes it from review lists and rating summaries for every caller.

## Requirements

### Requirement: Admins hide a review with an audited reason

`EngagementService.HideReview` SHALL be callable only by an authenticated caller holding the `admin` scope. It SHALL validate `review_id` and `reason` (1-500 characters after trimming whitespace), write one team-audit event, and only then mark the review hidden (`hidden_at`, `hidden_by`, `hide_reason`). A hidden review SHALL be excluded from ListReviews and from both rating summaries for every caller. The audit event SHALL have `action=review.hide`, `target_type=review`, `target_id` equal to the review id, `actor_id` equal to the admin's user id, and `metadata` holding `reason` and `listing_id`.

#### Scenario: Admin hides a review
- **WHEN** an admin calls HideReview with a valid review id and reason "spam link"
- **THEN** the call returns OK and the review no longer appears in ListReviews for any caller
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewHidesFromList`

#### Scenario: Hide removes the review from both summaries
- **WHEN** an admin hides a verified 1-star review among verified 5 and 3 star reviews
- **THEN** GetListingRatingSummary and GetShopRatingSummary both return `average_rating` 4.0 and `review_count` 2
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewUpdatesSummaries`

#### Scenario: Audit event carries the required fields
- **WHEN** an admin with user id "adm-1" hides review "r-1" of listing "L-1" with reason "spam link"
- **THEN** exactly one WriteAuditEvent call is made with action `review.hide`, target_type `review`, target_id "r-1", actor_id "adm-1" and metadata `reason`="spam link", `listing_id`="L-1"
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewAuditEventFields`

#### Scenario: Audit is called as the service principal
- **WHEN** the audit client sends WriteAuditEvent
- **THEN** the outgoing metadata principal is `service-team-engagement`, type `service`, scopes `audit.write`, and the admin's own principal metadata is absent
- **VERIFIED BY**: team-engagement `internal/upstream` `TestAuditClientUsesServicePrincipal`

#### Scenario: Audit is written before the hide
- **WHEN** HideReview runs
- **THEN** the audit call happens-before the repository hide call (recorded call order is audit, then hide)
- **VERIFIED BY**: team-engagement `internal/service` `TestHideReviewAuditBeforeHide`

#### Scenario: Audit failure leaves the review visible
- **WHEN** team-audit returns an error or times out during HideReview
- **THEN** the call returns `Unavailable`, the review's `hidden_at` is NULL and it still appears in ListReviews
- **VERIFIED BY**: team-engagement `internal/service` `TestHideReviewAuditFailureFailsClosed`

#### Scenario: No audit upstream configured
- **WHEN** HideReview runs with `UPSTREAM_AUDIT_ADDR` unset
- **THEN** the call returns `Unavailable` and the review stays visible
- **VERIFIED BY**: team-engagement `internal/service` `TestHideReviewNilAuditClientUnavailable`

#### Scenario: Hide failure after audit is Internal and retryable
- **WHEN** the audit write succeeds and the repository hide update fails, then the call is retried with the repository healthy
- **THEN** the first call returns `Internal`, the retry returns OK, the review is hidden, and the audit trail holds 2 `review.hide` events for the review (audit is at-least-once: the failed attempt and the retry)
- **VERIFIED BY**: team-engagement `internal/service` `TestHideReviewUpdateFailureInternalThenRetry`

#### Scenario: Hiding an already hidden review is idempotent
- **WHEN** an admin hides a review that is already hidden
- **THEN** the call returns OK, `hidden_by` and `hide_reason` keep their first values and no second audit event is written
- **VERIFIED BY**: team-engagement `internal/service` `TestHideReviewIdempotentNoSecondAudit`

#### Scenario: Unknown review
- **WHEN** an admin calls HideReview with an id that does not exist
- **THEN** the call returns `NotFound` and no audit event is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewNotFound`

#### Scenario: Empty or whitespace reason is rejected
- **WHEN** an admin calls HideReview with reason "" and then with reason "   "
- **THEN** both calls return `InvalidArgument`, no audit event is written and the review stays visible
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewEmptyReasonInvalidArgument`

#### Scenario: Reason length boundary
- **WHEN** an admin calls HideReview with a 500-character reason, and another review with a 501-character reason
- **THEN** the 500-character call returns OK and the 501-character call returns `InvalidArgument`
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewReasonLengthBoundary`

#### Scenario: Reason is trimmed before storing
- **WHEN** an admin calls HideReview with reason "  spam  "
- **THEN** `hide_reason` and the audit metadata reason are both "spam"
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewReasonTrimmed`

#### Scenario: Empty review id is rejected
- **WHEN** an admin calls HideReview with an empty `review_id`
- **THEN** the call returns `InvalidArgument`
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewEmptyIDInvalidArgument`

#### Scenario: Non-admin is denied at the service
- **WHEN** an authenticated buyer, a seller, and a service principal without the admin scope each call HideReview directly on engagement
- **THEN** each call returns `PermissionDenied`, no audit event is written and the review stays visible
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewNonAdminPermissionDenied`

#### Scenario: Authorization is checked before validation
- **WHEN** a buyer calls HideReview with an empty reason and an unknown review id
- **THEN** the call returns `PermissionDenied` (not `InvalidArgument` or `NotFound`) and no audit event is written
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewNonAdminInvalidReasonPermissionDenied`

#### Scenario: Anonymous caller is unauthenticated at the service
- **WHEN** a call with no forwarded principal reaches HideReview
- **THEN** the call returns `Unauthenticated`
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewAnonymousUnauthenticated`

#### Scenario: Hidden review is invisible to the admin too
- **WHEN** an admin lists reviews of the listing after hiding one
- **THEN** the hidden review is absent from the admin's ListReviews response
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewInvisibleToAdminList`

#### Scenario: Other review RPCs treat a hidden review as missing
- **WHEN** a buyer calls MarkReviewHelpful on a hidden review
- **THEN** the call returns `NotFound` and the review's `helpful_count` is unchanged
- **VERIFIED BY**: team-engagement `internal/handler` `TestMarkHelpfulHiddenReviewNotFound`

#### Scenario: Hide a review end to end
- **WHEN** an admin hides a review through the gateway with reason "spam link"
- **THEN** ListReviews through the gateway no longer returns it, the listing summary excludes it, and QueryAuditLog (admin) returns one event with action `review.hide`, target_id the review id, actor_id the admin's id and the reason in metadata
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "An admin hides a review with an audited reason"

#### Scenario: Re-hide writes no second audit event, end to end
- **WHEN** the admin hides the same review a second time through the gateway
- **THEN** the call succeeds and QueryAuditLog still returns exactly one `review.hide` event for that review
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "An admin hides a review with an audited reason"

#### Scenario: Buyer cannot hide a review, end to end
- **WHEN** a buyer calls HideReview through the gateway
- **THEN** the gateway returns `permission_denied` and the review stays listed
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "A non-admin cannot hide a review"

#### Scenario: Anonymous caller cannot hide a review, end to end
- **WHEN** an anonymous client calls HideReview through the gateway
- **THEN** the gateway returns `unauthenticated`
- **VERIFIED BY**: platform-e2e `features/engagement/review_integrity.feature` "A non-admin cannot hide a review"
