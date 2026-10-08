## Context

See `proposal.md` — *Why*. Current state (verified in code):

- `team-search` indexer: `ListingEventHandler` switches on `EventEnvelope.Type`. `ListingStockChanged` has a
  constant but no `case`; it reaches `default: return nil`. The version for every write is
  `envVersion(env)` = `occurred_at` in nanoseconds; `0` (no timestamp) disables the guard.
- Index writes: `Upsert` is a full replace with OpenSearch **external versioning** (409 = stale = no-op).
  `PartialUpdate` is a painless script that applies `fields` only if `params.version` is greater than
  `_source.version`, and answers 404 (treated as no-op) for a missing document, so it cannot resurrect a deleted
  listing. Both are AD2. The consumer wrapper retries, then parks on `<topic>.dlq` before advancing (AD1).
- Mapping (`indexMapping` const): `id, title, description, status, currency, price, category_id, seller_id,
  rating, version`. `EnsureIndex` creates the index only when absent. No `stock`.
- `ListingStockChanged` (proto `platform/listing/v1/listing.proto`): `listing_id`, `int32 stock`, repeated
  `variants`. Domain side (`BuildListingStockChangedEnvelope`, `NewStockEventBuilder`): `stock` is the base
  listing's post-change stock, `occurred_at = timestamppb.Now()` taken inside the stock-change transaction
  after the row update (so the row lock orders it), `event_id` = the outbox row id (stable across redelivery),
  record key = `listing_id`, so one listing's events share a partition and arrive in commit order.
- `Listing.stock` (field 10) is already on `ListingChanged`, but `toDoc` drops it.
- `SearchHit` = `{listing_id, score}`. Structured `filters` are a `map<string,string>` turned into `term`
  clauses on the same-named field, so `in_stock` needs explicit handling.
- In-flight `add-hybrid-retrieval-platform` (0/39) also changes the index: alias-addressed `listings-v{N}`
  (its task 3.1), v2 mapping with `knn_vector` (3.2), startup dimension check (3.3), and edits `search.proto`
  (task 1.1: `SearchMode` + `mode = 9` on the **request**). It must keep AD1 and AD2 working (its 4.4).

Constraints: Rule 3 (team-search owns its index; the event is the only input), Rule 4 (contract only in
platform-core, versioned, additive), Rule 5 (events on Kafka, unchanged), AD1, AD2.

## Goals / Non-Goals

**Goals:**

- Indexed stock converges to the domain's latest published stock under redelivery, reordering and replay.
- No new way to lose a record: failure semantics are exactly AD1.
- The smallest change that unblocks the e2e scenario and an `in_stock` filter, and that the v2 index work can
  absorb without rework.

**Non-Goals (design-level):** no change to the `version` semantics of existing events, no change to
`Suggest`, no per-variant stock (a variant-sum definition is a future domain-side decision), no stock-based ranking, no new index or alias.

## Decisions

### D1 — A dedicated `UpdateStock` index operation, not a generic `PartialUpdate` call

The handler gains `case listingStockChangedType` that validates the payload and calls a new
`Index.UpdateStock(ctx, id, stock int32, stockVersion int64)`. It runs a painless update whose guard compares
`stock_version`, not `version`:

```
if (ctx._source.stock_version != null && ctx._source.stock_version >= params.sv) { ctx.op = 'noop'; }
else { ctx._source.stock = params.stock; ctx._source.stock_version = params.sv; }
```

No `upsert` clause, so a 404 (missing/deleted document) is a successful no-op exactly like `PartialUpdate`.
*Alternative:* reuse `PartialUpdate` with `{"stock": n, "version": v}`. Smallest diff, but see D2: it forces
stock onto the shared `version`. *Alternative:* a separate `listing_stock` index joined at query time. Rejected:
a second index to keep consistent and a join the engine does not do; it would also not compose with the v2
alias swap.

### D2 — Stock has its own version (`stock_version`), from the envelope `occurred_at`; it never touches `version`

Two observations drove this. (1) With the shared `version`, a price event at T3 makes a not-yet-delivered stock
event at T2 (T2 < T3) look stale, so a legitimate stock change is dropped. (2) A stock event advancing
`version` makes a not-yet-delivered ListingChanged/base-info event at an earlier time look stale, so a seller
edit is dropped. Both are plausible across redelivery and replay (events of different types for one listing
are separate outbox rows). Stock is a different aggregate of facts than title/price/status, so it gets its own
ordering axis. The clock is the same one the rest of the indexer already trusts (`occurred_at` nanos, taken
under the domain row lock), so no new assumption is introduced. `stock_version` is `0`/absent until the first
stock event; `occurred_at` missing is **not** treated as "guard off" for stock: it is a malformed event
(spec), so a wrong-order write cannot slip through.
*Alternative considered:* a per-listing monotonic counter in the event. Needs a proto and domain change and a
new column; not needed while the row-lock-ordered timestamp suffices. Noted as the upgrade path if clock skew
between domain replicas ever matters (Risks).

### D3 — Full `ListingChanged` upserts carry stock forward; `Listing.stock` only seeds

`Upsert` is a full replace, so adding `stock` to `ListingDoc` naively would make every seller edit overwrite
newer stock with whatever `Listing.stock` said when the edit was made, and would drop `stock_version`. Rule:

- `ListingDoc.Stock` is `*int32` with `omitempty`, `StockVersion int64` with `omitempty`.
- In the `ListingChanged` path, before the external-versioned index call, read the existing `stock` and
  `stock_version` (one GET by id). If `stock_version > 0`, copy both onto the doc (existing stock wins). If the
  document is absent or has no `stock_version`, set `stock = Listing.stock` and leave `stock_version` unset,
  so any later stock event overrides the seed.
- The read-before-write is safe because one listing's events share a partition and the indexer handles a
  partition sequentially; a task verifies that assumption (task 2.3) rather than trusting it.
*Alternative:* turn `Upsert` into a scripted upsert that preserves the stock fields server-side. Atomic, but it
rewrites the AD2 mechanism for all events for one field; larger blast radius and it would have to be redone for
the v2 index. *Alternative:* ignore `Listing.stock` entirely. Rejected: a listing whose first stock event was
ignored as an unknown document would show no stock at all until the next reservation.

### D4 — Add the fields to the current mapping by idempotent put-mapping at startup

`EnsureIndex` today does nothing when the index exists, so adding fields only to the create-mapping would leave
the live index unmapped (OpenSearch dynamic mapping would guess `long` for `stock`, but `stock_version`/`stock`
must be deterministic and `integer`). After the exists-check, `EnsureIndex` issues
`PUT /{index}/_mapping {"properties":{"stock":{"type":"integer"},"stock_version":{"type":"long"}}}`, which is
a no-op when the mapping already matches and additive otherwise (no reindex, no downtime). The same two fields
go into the create-mapping for fresh clusters. A conflicting pre-existing mapping (for example a dynamically
mapped `stock` of another type) fails startup loudly.
*Alternative:* rely on dynamic mapping. Rejected for determinism. *Alternative:* a new index generation plus
reindex now. Rejected: that is exactly the v2 work; doing it twice buys nothing.

### D5 — Composition with `add-hybrid-retrieval-platform`: this change first; v2 inherits the fields

Ordering: land this change first against the current single index. It is small, touches the consumer, one
index method and the mapping, and it unblocks an e2e scenario already waiting. The hybrid change then builds on
it. Required coordination (task 6.1, done: hybrid's tasks 3.2/3.3 and D9 were amended to carry `stock` and `stock_version`):

- The v2 mapping in hybrid task 3.2 SHALL include `stock` (`integer`) and `stock_version` (`long`) next to
  `version`, and its startup contract check (3.3) should assert they exist.
- Hybrid's D9 rebuilds by replaying `listing.events` into `listings-v2` (not `_reindex`), so stock is
  repopulated by the replayed `ListingChanged` seed plus the stock events; they share the topic, and the
  `stock_version` guard makes the catch-up overlap with the live indexer idempotent.
- `UpdateStock` and the stock carry-forward in `Upsert` must address the **alias** once hybrid 3.1 lands; both
  use `o.name`, which hybrid repoints at the alias, so no stock-specific change is needed.
- The proto: hybrid task 1.1 adds `mode` to `SearchListingsRequest`; this change adds `stock` to `SearchHit`
  (a different message, field 3). No field-number or message clash. Whichever lands second re-vendors again
  (task 1.2 here / hybrid 1.2); both are mechanical.
- The `in_stock` filter is a structured filter and therefore applies uniformly to every strategy hybrid adds
  (its filtered kNN clause takes the same filter list), which is another reason to express it in the shared
  filter builder and not in the lexical path only (task 3.2).
*Alternative:* fold both into one change or wait for v2. Rejected: v2 is 0/39 and large; stock is correctness
for an already-shipped domain event.

### D6 — Expose `stock` as an optional field on `SearchHit`; `in_stock` as a filter key

```proto
message SearchHit {
  string listing_id = 1;
  float score = 2;
  optional int32 stock = 3;   // present iff the read-model holds a stock for the listing
}
```

`optional` gives presence, so "unknown" is distinguishable from zero (spec). It is additive, so `buf breaking`
passes and existing callers ignore it. The handler fills it from `_source.stock`. The filter key `in_stock` is
consumed in the index filter builder and never emitted as a literal `term` on a nonexistent field; it becomes
`range stock > 0`, so unknown-stock documents do not match (spec).
Repos touched by the proto change: `platform-core` (source), then vendored/regenerated copies in
`team-search`, `team-gateway` (forwarder passes the message through unchanged), `team-frontend` (vendored
generated client; no UI change in this change) and `team-ai` (vendors the search proto per hybrid task 1.2).
*Alternative:* index-only for now. Cheaper, but the e2e helper needs to observe the value and there is no other
honest way to test "search shows updated stock" without reading OpenSearch directly from the e2e suite (a Rule 3
leak in the test platform). *Alternative:* a dedicated `GetListingStock` RPC. Rejected: a second call per hit
page to show a number the search already holds.

### D7 — Failure and poison handling reuse AD1 unchanged

`UpdateStock` returns an error on any non-404 OpenSearch failure; the existing consumer wrapper retries with
backoff, then parks on the DLQ and advances. Validation errors (empty `listing_id`, negative stock, nil
`occurred_at`, payload unmarshal failure) are returned as errors too, so a poison stock record is parked rather
than silently acknowledged (the current `default: return nil` behaviour is right for *foreign* types and stays
for them). The 404 path is explicitly success (spec: unknown/deleted listing never blocks the partition).

## Risks / Trade-offs

- [Clock skew between domain replicas could order two stock events wrongly] → `occurred_at` is taken inside the
  row-locked transaction, so the skew window is the clock difference between consecutive transactions on the
  same listing; accepted, consistent with the indexer's existing trust in `occurred_at`; upgrade path is a
  per-listing counter in the event (D2).
- [A stock event can be dropped as "unknown listing" if it is delivered before the listing's create]
  → same partition and commit order make this a non-case in steady state; if it happens, D3 seeds stock from
  `Listing.stock` when the create is applied, and any later stock event fixes it.
- [Replay from offset 0 applies ListingChanged seeds and stock events interleaved] → both orders converge:
  seed only when no `stock_version`; stock events guarded by `stock_version`.
- [`variants`-based listings: event `stock` is the base stock only] → indexed stock mirrors that number; the
  spec promises the domain's published value, not a computed total; per-variant stock is a non-goal.
- [Existing documents have no stock until the next event or a replay] → spec says "unknown", `in_stock=true`
  excludes them, hits omit the field; replay (Migration Plan) fills them.
- [Read-before-write in `Upsert` adds a GET per ListingChanged] → one indexed GET by id on a low-volume event
  type; the alternative (scripted upsert) is larger and rejected (D3).
- [Stock changes are far more frequent than listing edits, each a refresh-true scripted update]
  → `Refresh: "true"` is what the existing partial update does; if load shows it matters, batch/relax refresh
  as a separate change. Not designed away here.

## Migration Plan

1. `platform-core`: add `optional int32 stock = 3` to `SearchHit`; `buf lint` + `buf breaking`.
2. Re-vendor + regenerate in `team-search`, `team-gateway`, `team-frontend`, `team-ai`. All additive.
3. `team-search`: deploy the indexer and query server together with the new mapping call. Startup put-mapping
   is idempotent; order between the two processes does not matter (both call `EnsureIndex`).
4. Fill existing documents: no dedicated backfill. Replay `listing.events` (see the runbook note, task 8.3):
   stop the indexer, reset its consumer group offset to earliest (for example
   `kafka-consumer-groups.sh --group <search-indexer-group> --topic listing.events --reset-offsets --to-earliest --execute`
   while the group is inactive) or start a new group from earliest into a scratch index, then restart.
   `ListingChanged` seeds plus stock events reproduce state. Until a document's next stock event or a replay,
   its stock is unknown (field absent; `in_stock=true` excludes it).
5. Gateway and frontend need no deploy ordering: the field is optional and ignored until used.
6. Rollback: revert the indexer; the two extra fields stay in the mapping and documents harmlessly (old code
   never reads them and the old `Upsert` replace simply drops them, to be re-filled by a later replay).

## Open Questions

- Should the frontend show an "out of stock" badge from `hit.stock` now, or in a follow-up? (UI only; this
  change ships the data.)
