## Context

`CreateReview` and `HideReview` in `team-engagement/internal/store/review.go` already enqueue `ReviewCreated` and
`ReviewHidden` through `enqueueFact` inside `postgres.WithTx`. Migration 0014 shows how to build envelopes in SQL
(`pg_temp.pb_*` helpers) for outbox rows.

## Decisions

- Compute the aggregate in the writing transaction, after the insert or update, with
  `SELECT rating, COUNT(*) FROM reviews WHERE listing_id = $1 AND verified_purchase AND hidden_at IS NULL GROUP BY rating`
  and the same `aggregateRows` arithmetic as `GetRatingSummary`. One shared helper keeps the two identical.
  Two concurrent writers on one listing may each see the other's row or not; the later commit's event is computed
  after its own write and the relay orders per key by `seq`, so the final event per listing converges on the final
  state only if the later transaction saw both writes. Decided (C5): the helper takes
  `pg_advisory_xact_lock(hashtext(listing_id))` before the aggregate read, in `CreateReview` and `HideReview`
  alike, so writers on one listing serialise.
- The event has no principal (C3): the row is built with a principal-less envelope instead of `FactRow`'s
  context principal, and the backfill writes none.
- Migration 0015 deploys together with the new writer binary (C9); the next write on a listing recomputes the
  absolute value, so a gap between the two self-heals.
- Down migration identifies backfill rows by the deterministic event-id pattern: `event_id` equal to
  `md5('listing-rating-backfill:' || aggregate_id)` formatted as a UUID, with `status <> 'published'`.
- Emit after `ReviewCreated` / `ReviewHidden` in the same transaction; no event when the write is a no-op
  (duplicate review, already hidden, review not found).
- Backfill in SQL like 0014, because migrations run without Go code. `average_rating` is a protobuf `double`
  (fixed64, wire type 1), so the SQL needs a little-endian 8-byte encoding of the `float8` value
  (`float8send` reversed byte-wise). `changed_at` is the migration time. The average uses `sum(rating)::float8 /
  count(*)`, the same IEEE double as Go's `float64(sum)/float64(count)`.
- Backfill event ids are deterministic (`md5('listing-rating-backfill:' || listing_id)` as UUID text) with
  `ON CONFLICT (event_id) DO NOTHING`, so a re-run adds nothing.
- Listings whose reviews are all hidden or unverified emit `0 / 0`, so a stale non-zero value in search is
  corrected.

## Risks

- Backfill rows share `available_at = now()`; the relay drains them in `seq` order, so a live event written during
  the drain has a later `seq` and wins. Acceptable because values are absolute.
- Hand-encoded protobuf can drift from the contract; the migration test decodes the payload with the generated
  `ListingRatingChanged` type.
