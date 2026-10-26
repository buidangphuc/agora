## MODIFIED Requirements

### Requirement: The engagement consumer records a review_hidden fact

The engagement consumer SHALL map an `engagement.events` envelope of type `platform.engagement.v1.ReviewHidden` to one `engagement_facts` row with `fact = review_hidden`, the payload `review_id` and `listing_id`, empty `user_id` and `seller_id`, NULL `rating`, `event_id` equal to the envelope `event_id`, and `occurred_at` equal to the payload `hidden_at` (the envelope `occurred_at` when `hidden_at` is unset). The original `review_created` row SHALL be left untouched.

#### Scenario: Analytics review hidden: a ReviewHidden envelope maps to a review_hidden fact
- **WHEN** an envelope of type `platform.engagement.v1.ReviewHidden` with review id "r-1", listing id "l-1" and `hidden_at` 2026-10-18T10:00:00Z is mapped
- **THEN** the record has `Fact` "review_hidden", `ReviewID` "r-1", `ListingID` "l-1", empty `UserID` and `SellerID`, `Rating` 0 and `OccurredAt` 2026-10-18T10:00:00Z, and no error is returned
- **VERIFIED BY**: team-analytics `internal/events` `TestEngagementFactFromEnvelope`

#### Scenario: Analytics review hidden: a produced ReviewHidden reaches the warehouse
- **WHEN** a ReviewCreated and then a ReviewHidden envelope for the same review, each with a unique event id, are produced to engagement.events
- **THEN** within 30 seconds the warehouse holds one `review_created` row and one `review_hidden` row (with that review id and listing id, NULL rating), and the DLQ topic holds no record for either event id
- **VERIFIED BY**: platform-e2e `features/analytics/review_hidden.feature` "Analytics review hidden: a produced ReviewHidden reaches the warehouse"

#### Scenario: Analytics review hidden: the review_created row is kept
- **WHEN** a `review_hidden` fact is written for a review that already has a `review_created` row
- **THEN** the `review_created` row still exists unchanged (rating 4 stays 4) and the table holds two rows for the pair
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestReviewHiddenKeepsReviewCreated`

#### Scenario: Analytics review hidden: hidden_at missing falls back to the envelope time
- **WHEN** a ReviewHidden envelope with no `hidden_at` and envelope `occurred_at` 2026-10-18T09:00:00Z is mapped
- **THEN** the record `OccurredAt` equals 2026-10-18T09:00:00Z
- **VERIFIED BY**: team-analytics `internal/events` `TestReviewHiddenOccurredAtFallback`

#### Scenario: Analytics review hidden: a replayed event is stored once
- **WHEN** the same ReviewHidden envelope (same event id) is delivered twice
- **THEN** the warehouse holds exactly one `review_hidden` row for that event id
- **VERIFIED BY**: team-analytics `internal/warehouse/duckdb` `TestReviewHiddenIdempotent`

#### Scenario: Analytics review hidden: a corrupt payload is dead-lettered
- **WHEN** a ReviewHidden envelope whose payload bytes are not valid protobuf, or which has no event id, is processed in a batch with good records
- **THEN** the mapping returns an error, the record is dead-lettered by the SDK to `ENGAGEMENT_DLQ_TOPIC` carrying the SDK `dlq-*` headers (not an `error` header), and the other records are still written
- **VERIFIED BY**: team-analytics `internal/events` `TestEngagementFactFromEnvelopeRejects`
- **VERIFIED BY**: team-analytics `internal/events` `TestMalformedEngagementRecordIsPermanent`
- **VERIFIED BY**: team-analytics `internal/app` `TestMalformedEngagementRecordIsDeadLetteredAndGroupMovesOn`

#### Scenario: Analytics review hidden: existing engagement types are unchanged
- **WHEN** envelopes of the five existing engagement types and an unknown type "platform.engagement.v1.Nope" are mapped
- **THEN** the five map to the same facts as before this change and the unknown type still returns `ErrUnknownEngagementType`
- **VERIFIED BY**: team-analytics `internal/events` `TestEngagementFactFromEnvelope`
- **VERIFIED BY**: team-analytics `internal/events` `TestEngagementFactFromEnvelopeRejects`
