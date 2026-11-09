## Why

The search rating filter (initiative discovery-seo, story 3) needs each listing's rating inside the search document.
team-search has no database and must not call team-engagement per hit, so team-engagement publishes the number
itself (decision D1): `ListingRatingChanged` with absolute values on `engagement.events`. Existing listings must not
show "no rating" until their next review, so the migration backfills one event per reviewed listing (decision D2).

initiative: discovery-seo

## What changes

- New fact `ListingRatingChanged{listing_id, average_rating, review_count, changed_at}` (contract from
  `discovery-seo-contract`) written to the outbox, key = listing id, in the same transaction as each rating-changing
  write:
  - `CreateReview` (always a verified purchase today);
  - `HideReview` when it actually hides a review (`hidden_at` was NULL).
- There is no unhide RPC today, so no unhide path is added. Any future unhide SHALL emit the same event; this is
  recorded as a requirement on the writer, not built now.
- The values are computed inside the writing transaction with the same predicate as `GetListingRatingSummary`
  (`verified_purchase AND hidden_at IS NULL`), so the event equals what the RPC returns right after the commit.
  `average_rating` is 0 when `review_count` is 0 (hiding the last review).
- Migration `0015_listing_rating_backfill`: enqueues one `ListingRatingChanged` envelope per listing with at least
  one review row (including listings whose reviews are all hidden, which emit `0 / 0`), as pending rows of
  `engagement_outbox_events`, idempotent on re-run. Down migration removes only unpublished backfill rows.
- `ReviewCreated` and `ReviewHidden` are unchanged and still written.
- The envelope of `ListingRatingChanged` has no principal, in live writes and in the backfill (C3).
- Hide rule: every `HideReview` that moves a review from visible to hidden emits one event, whether or not the
  visible summary changed (values are absolute, so an unchanged summary is harmless); a hide that changes no row
  emits none.
- Deploy: migration 0015 ships and deploys together with the new writer binary (C9). Reviews written between the
  migration and the new binary starting are covered because the next write on that listing recomputes and publishes
  the absolute value, so any gap self-heals.

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `engagement-facts`: adds "Listing rating changes are published as absolute values".

## Impact

- `team-engagement/internal/store/review.go` (CreateReview, HideReview), `internal/store/storetest/review.go` (fake
  repository parity), new `team-engagement/migrations/0015_listing_rating_backfill.{up,down}.sql`,
  `migrations/migration_test.go`, `team-engagement/FEATURES.yaml` (by e2e author).
- Regenerated stubs for `ListingRatingChanged` come from `discovery-seo-contract`.
- Consumer: team-search indexer (slice `search-bestseller-rating`); no other consumer is changed here.

## NFR

- Added write latency: one aggregate query (`SELECT rating, COUNT(*) ... GROUP BY rating`, uses the listing index)
  and one outbox insert inside the existing transaction; `CreateReview` and `HideReview` p95 must not regress more
  than 10% on the local stack. Measured, non-gating: no scenario asserts it.
- Delivery: at least once, in `seq` order per listing key, through the existing relay; a broker outage does not park
  events (existing `engagement-facts` rule). Event reaches `engagement.events` within 5 s of commit when Kafka is up
  (asserted by the e2e scenario "A new review publishes the new average and count"; the 30 s search budget of the
  initiative leaves the rest to team-search).
- Backfill: up to 100,000 reviewed listings in one migration run in under 60 s; each row is one small insert. Measured, non-gating: no scenario asserts it.
- Redelivery and reordering are harmless because values are absolute; the last event per listing key wins.

## Data classification

| field | personal? | purpose | consent basis | retention |
|---|---|---|---|---|
| `listing_id` | no | key of the aggregate | n/a | outbox row: published rows are deleted after 7 days by the hourly job in `internal/app` |
| `average_rating` | no (aggregate over at least 0 reviewers) | search filter and facet | n/a | outbox 7 days after publish, then only in the search index |
| `review_count` | no | search display and filter | n/a | outbox 7 days after publish, then only in the search index |
| `changed_at` | no | ordering and diagnostics | n/a | outbox 7 days after publish, then only in the search index |

No reviewer id, review id, seller id, comment, media or moderator identity travels in the event. The envelope
carries no principal for `ListingRatingChanged`, live or backfill (decision C3), so no reviewer or admin id leaves
the service through it. Outbox rows follow the existing `engagement_outbox_events` retention: published rows are
deleted after 7 days. DPO: no new personal data.

## Threat model

- Assets: integrity of the public rating used for ranking and filtering; review text privacy.
- Abuse cases:
  - A moderator-hidden or non-verified review inflating the rating in search: prevented by using the
    `GetListingRatingSummary` predicate in the same transaction.
  - A forged event on `engagement.events` skewing ratings: out of scope here; topic write access is broker-level and
    unchanged.
  - Leaking review text or moderator identity through the event: the message has no such fields.
  - Hide of an already hidden review re-emitting events and flooding search: no event when no row changed.
- Authz rule: unchanged. `CreateReview` still requires a verified buyer of the order; `HideReview` still requires an
  admin and the audit gate. This slice adds no RPC and no new permission. An unauthorised or failed call writes no
  event because the fact lives in the same transaction as the write.
