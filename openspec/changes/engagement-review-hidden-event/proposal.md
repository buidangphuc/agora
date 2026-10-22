## Why

When an admin hides a review, team-engagement updates the row (`hidden_at`, `hidden_by`, `hide_reason`) but publishes nothing. team-analytics and the featurestore therefore keep counting hidden reviews. The locked initiative `privacy-audit-hardening` (story "Analyst") requires a `ReviewHidden` outbox fact on `engagement.events`. The proto message is provided by `pah-contract`; analytics consumption by `analytics-review-hidden`. This slice emits the fact.

initiative: privacy-audit-hardening

## What Changes

- **Repository** (`team-engagement/internal/repository/review.go`): `PostgresReviewRepository.HideReview` runs the UPDATE and the outbox insert in one transaction (`pgx.BeginFunc`, as `CreateReview` does). The UPDATE gains `RETURNING listing_id, hidden_at`; when exactly one row changed it calls `enqueueFact(ctx, tx, listingID, &engagementv1.ReviewHidden{ReviewId, ListingId, HiddenAt})` (key = listing id, same as `ReviewCreated`). `hidden_at` in the event is the value stored by the UPDATE. Interface signature `HideReview(ctx, id, adminID, reason) (changed bool, err error)` is unchanged; the in-memory repository needs no outbox.
- **Idempotence**: re-hiding an already hidden review changes 0 rows and enqueues nothing (service already returns early on `HiddenAt != nil`; the `hidden_at IS NULL` guard covers concurrent admins).
- **Service/handler**: no change. Order stays: validate, load, already-hidden no-op, audit write (fail closed), hide + fact.
- **Event content**: `review_id`, `listing_id`, `hidden_at` only. No `hidden_by`, no `hide_reason`, no review text, no user id.
- **No proto, migration, gateway or compose change.** The proto message and the vendored copy in team-engagement come from `pah-contract`.
- **E2E** (`platform-e2e`): extend `features/engagement/engagement_facts.feature` style (new feature file `review_hidden_event.feature`) with the Kafka reader.

## Deploy order

1. `pah-contract` (proto) and `analytics-review-hidden` (team-analytics consumes `ReviewHidden`) first. Unknown types are dead-lettered by analytics, so engagement MUST NOT deploy before analytics.
2. Then team-engagement (this slice).
Rollback: reverting team-engagement stops new facts; facts already in the outbox are still delivered and analytics keeps handling them.

## NFR

- Latency: one extra INSERT in the hide transaction; p99 HideReview change <= 10 ms over the audit call's existing cost.
- Availability: unchanged. The outbox relayer publishes asynchronously (at-least-once); a Kafka outage delays the event but does not fail HideReview. If the audit write fails the call is `Unavailable` and no fact is written.
- Limits: payload under 200 bytes; one fact per effective hide; consumers must tolerate duplicates (at-least-once).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `review_id`, `listing_id` | pseudonymous object ids, no user id | let analytics and features stop counting the review | legitimate interest (data accuracy) | analytics event retention (set by team-analytics); outbox rows follow existing outbox purge |
| `hidden_at` | metadata | ordering/effective time | same | same |
| `hidden_by`, `hide_reason` | personal-light (admin id, free text) | stay in the reviews row and in the `review.hide` audit event only | existing Q2 basis | unchanged; NOT copied to the event |

## Threat model

- **Assets:** analytics integrity (hidden review keeps skewing ratings and features), `engagement.events` topic, admin identity and moderation reason.
- **Abuse cases:** (1) non-admin triggers a fact: HideReview stays admin-only (existing `RequireAdmin`, gateway admin procedure), so no new path. (2) Replay or double hide inflating events: guarded by `hidden_at IS NULL` plus idempotent no-op. (3) Leaking moderator identity or reason to analytics: excluded by contract. (4) Fact lost between DB and Kafka: same transaction (outbox), so never hidden-without-fact.
- **Authz rule:** admin only (unchanged). Emission is a side effect of a successful admin hide; callers cannot supply listing id or timestamp.
