## Why

Initiative `privacy-audit-hardening` (locked 2026-10-18, Clarification Q8) makes hidden reviews stop counting in analytics. Today `team-analytics` keeps a `review_created` fact for every review and has no way to learn that an admin hid it. The engagement consumer (`team-analytics/internal/consumer/engagement.go`) dead-letters any envelope type it does not know (`ErrUnknownEngagementType`), so the new `platform.engagement.v1.ReviewHidden` event (added by `pah-contract`) would land on the DLQ and be lost. This slice teaches the consumer the new type and records a `review_hidden` fact that the featurestore slice (`featurestore-hidden-review-exclusion`) can join on.

initiative: privacy-audit-hardening

## What Changes

- **team-analytics** `internal/consumer/engagement.go`: new constant `ReviewHiddenEventType = "platform.engagement.v1.ReviewHidden"` and a switch case that decodes `engagementv1.ReviewHidden` into an `EngagementFactRecord` with `Fact = review_hidden`, `ReviewID`, `ListingID` and `OccurredAt`. Existing five mappings and the dead-letter path are unchanged.
- **team-analytics** `internal/warehouse/warehouse.go`: new `FactReviewHidden = "review_hidden"`; `EngagementFactRecord` gains `ReviewID`.
- **team-analytics** DuckDB `engagement_facts`: a nullable `review_id` column (additive migration, `ADD COLUMN IF NOT EXISTS`, idempotent on reopen) is written for both `review_created` (from `ReviewCreated.review_id`) and `review_hidden` rows so the featurestore can anti-join hidden reviews exactly. `rating` stays NULL for `review_hidden`; `user_id` and `seller_id` are empty.
- **Parquet export**: `engagement_facts.parquet` carries `review_hidden` rows and the `review_id` column with no export code change beyond the column list.
- **team-analytics README**: the Consume row and the `engagement_facts` table row list `ReviewHidden` / `review_hidden` and `review_id`.
- **platform-e2e**: feature `tests/e2e/features/analytics/review_hidden.feature` with bindings; produces a `ReviewHidden` envelope on `engagement.events` and reads the DuckDB warehouse.
- Not changed: proto (`pah-contract`), engagement emission (`engagement-review-hidden-event`), featurestore SQL, BigQuery adapter (engagement facts are DuckDB only), the original `review_created` row (never deleted or edited).

## Impact

- Affected specs: new capability `analytics-review-hidden`. Additive.
- Depends on `pah-contract`: vendored proto copy in team-analytics must carry `ReviewHidden` (run `make proto` in team-analytics before coding).
- **Deploy order**: this slice MUST be deployed before `engagement-review-hidden-event`. If engagement emits `ReviewHidden` first, the old consumer dead-letters it and the Kafka offset advances; those hides are not recovered without replaying the DLQ. `featurestore-hidden-review-exclusion` ships after this slice (it reads the new fact/column).

## NFR

- Latency/throughput: one extra switch case; no extra I/O. Batch behaviour (write batch idempotent, offset commit after write and DLQ publish) is unchanged.
- Availability: a malformed `ReviewHidden` payload is dead-lettered like any bad record and does not block the batch. Re-delivery of the same envelope writes one row (dedupe on `event_id`).
- Limits: none new.
- Accepted limitation: `review_created` rows written before this deploy keep `review_id` NULL and cannot be matched, so a later hide of a pre-deploy review is recorded but not excluded by an exact anti-join. No backfill.

## Data classification

- `review_id`, `listing_id` (ReviewCreated, ReviewHidden): business identifiers, no user id, no free text. Purpose: keep hidden reviews out of popularity/ALS features and keep analytics truthful. Consent basis: first-party service operation (no user-level data added). Retention: warehouse `engagement_facts` policy (unchanged).
- `occurred_at`: moderation timestamp. Same retention.
- No personal data field is introduced. The admin who hid the review is NOT carried in the event or stored here (it lives in team-audit).
- E2E data: synthetic ids prefixed `e2e-prh-`.

## Threat model

- Assets: integrity of engagement facts that drive recommendations; consumer availability.
- Abuse cases:
  - Forged ReviewHidden on `engagement.events` suppresses a competitor's reviews in features. Mitigation: trust boundary is the topic (only service producers write); unchanged. No new authz at this layer.
  - Poison/malformed payload stalls the consumer. Mitigation: decode error is dead-lettered, batch continues.
  - Replay duplicating hide facts. Mitigation: `event_id` dedupe.
  - ReviewHidden consumed by an old consumer. Mitigation: deploy order above.
- Authz rule: none (no RPC); Kafka topic ACLs unchanged.

## Open items

- Legacy `review_created` rows (pre-deploy) have NULL `review_id`; hides of those reviews are not excluded downstream (accepted by orchestrator decision, round 2; for the human list at LOCK).
- `occurred_at` for `review_hidden` is taken from `hidden_at` when set, else the envelope `occurred_at` (assumption).
- Un-hiding a review is not in the contract; no `review_unhidden` fact.
