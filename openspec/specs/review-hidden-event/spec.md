# review-hidden-event Specification

## Purpose
Hiding a review publishes one ReviewHidden event on engagement.events in the same transaction as the hide.

## Requirements

### Requirement: Hiding a review publishes ReviewHidden in the same transaction

When `EngagementService.HideReview` hides a review, team-engagement SHALL, in the same database transaction as the hide, enqueue exactly one `ReviewHidden{review_id, listing_id, hidden_at}` outbox fact keyed by the listing id and published on `engagement.events` in the standard EventEnvelope. The fact SHALL NOT contain the admin id, the hide reason, review text or any user id. Hiding an already hidden review SHALL enqueue nothing.

#### Scenario: Review hidden event: effective hide enqueues one fact
- **WHEN** an admin hides a visible review through the repository against Postgres
- **THEN** exactly 1 outbox row of type `platform.engagement.v1.ReviewHidden` exists with key equal to the review's listing id and a payload whose `review_id`, `listing_id` and `hidden_at` equal the stored review row values
- **VERIFIED BY**: team-engagement `internal/repository` `TestHideReviewEnqueuesReviewHidden`

#### Scenario: Review hidden event: payload carries no moderator data
- **WHEN** a review is hidden with admin id "admin-1" and reason "spam link"
- **THEN** the serialized fact contains neither "admin-1" nor "spam link" and has only fields 1 to 3 set
- **VERIFIED BY**: team-engagement `internal/repository` `TestReviewHiddenFactOmitsAdminAndReason`

#### Scenario: Review hidden event: fact and hide are atomic
- **WHEN** the outbox insert fails while an admin hides a review
- **THEN** HideReview returns an error and the review row still has `hidden_at` NULL (0 hidden rows, 0 outbox rows)
- **VERIFIED BY**: team-engagement `internal/repository` `TestHideReviewRollsBackWhenEnqueueFails`

#### Scenario: Review hidden event: re-hide emits nothing new
- **WHEN** an admin hides a review that is already hidden
- **THEN** HideReview returns success, `hidden_at`, `hidden_by` and `hide_reason` are unchanged and the outbox still holds exactly 1 `ReviewHidden` row for that review
- **VERIFIED BY**: team-engagement `internal/repository` `TestHideReviewTwiceEnqueuesOnce`

#### Scenario: Review hidden event: concurrent hides emit once
- **WHEN** two admins hide the same visible review concurrently
- **THEN** the outbox holds exactly 1 `ReviewHidden` row for that review
- **VERIFIED BY**: team-engagement `internal/repository` `TestHideReviewConcurrentEnqueuesOnce`

#### Scenario: Review hidden event: unknown review emits nothing
- **WHEN** an admin calls HideReview with a review id that does not exist
- **THEN** the call returns `NotFound` and the outbox gains 0 rows
- **VERIFIED BY**: team-engagement `internal/repository` `TestHideReviewUnknownEnqueuesNothing`

#### Scenario: Review hidden event: audit failure emits nothing
- **WHEN** an admin calls HideReview while the team-audit write fails or no audit upstream is configured
- **THEN** HideReview returns `Unavailable`, the review stays visible and the outbox gains 0 rows
- **VERIFIED BY**: team-engagement `internal/service` `TestHideReviewAuditFailureNoFact`

#### Scenario: Review hidden event: non-admin cannot trigger a fact
- **WHEN** a buyer, a seller or an anonymous caller calls HideReview
- **THEN** the call is refused with `permission_denied` or `unauthenticated` and the outbox gains 0 rows
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewNonAdminNoFact`

#### Scenario: Review hidden event: existing hide behaviour is preserved
- **WHEN** an admin hides a review and then lists the listing's reviews and summaries
- **THEN** the review is absent from the list, both rating summaries exclude it, and one `review.hide` audit event was written before the hide
- **VERIFIED BY**: team-engagement `internal/handler` `TestHideReviewHidesFromList` and `TestHideReviewUpdatesSummaries` (existing, unchanged)

### Requirement: ReviewHidden is observable on engagement.events and recorded by analytics

With the e2e stack running, an admin hide through the gateway SHALL produce one `ReviewHidden` envelope on `engagement.events` and team-analytics SHALL record a `review_hidden` event for it.

#### Scenario: Review hidden event: admin hide appears on engagement.events
- **WHEN** an admin hides a buyer's review through the gateway
- **THEN** within 30 s exactly one `ReviewHidden` envelope for that review id and its listing id appears on `engagement.events`, keyed by the listing id, and its payload does not contain the hide reason
- **VERIFIED BY**: platform-e2e `engagement/review_hidden_event.feature` "Hiding a review publishes a fact"

#### Scenario: Review hidden event: repeated hide publishes nothing new
- **WHEN** an admin hides the same review twice through the gateway
- **THEN** exactly one `ReviewHidden` envelope for that review appears on `engagement.events`
- **VERIFIED BY**: platform-e2e `engagement/review_hidden_event.feature` "A repeated hide publishes nothing new"

#### Scenario: Review hidden event: a buyer cannot hide a review
- **WHEN** a buyer calls HideReview through the gateway
- **THEN** the gateway returns 403 permission_denied (not an admin, missing scope) and no `ReviewHidden` envelope for that review appears on `engagement.events` within 10 s
- **VERIFIED BY**: platform-e2e `engagement/review_hidden_event.feature` "A buyer cannot hide a review"

#### Scenario: Review hidden event: analytics records review_hidden
- **WHEN** an admin hides a review through the gateway
- **THEN** within 30 s team-analytics holds one `review_hidden` record for that review id and listing id, using the query surface defined by `analytics-review-hidden`
- **VERIFIED BY**: platform-e2e `engagement/review_hidden_event.feature` "Analytics records the hidden review"
