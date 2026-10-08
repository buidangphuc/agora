## Why

`team-domain` now emits `ListingStockChanged` (EventEnvelope on `listing.events`, key `listing_id`) through the
transactional outbox, in the same transaction as every reserve, release and TTL-sweep. `team-search` never
applies it. Verified in code:

- `internal/consumer/listing.go` declares `listingStockChangedType` but has no `case` for it, so the event falls
  to `default: return nil // not ours; ignore`. The record is acknowledged and dropped.
- The OpenSearch `listings` mapping (`internal/index/opensearch.go`) has no `stock` field, and `ListingDoc`
  and `toDoc` ignore `Listing.stock` even though the proto carries it.
- `SearchHit` returns only `listing_id` and `score`, so nothing downstream can see stock.

The result: the stock event the domain team built is a dead letter, the read-model cannot answer "is it in
stock", and the `inventory-commit-and-idempotent-release` e2e scenario "search shows updated stock" cannot pass
(its task 3.1 was moved here). The e2e helper `SearchService.indexed_stock` is already written against a
`stock` value per hit.

## What Changes

- **team-search** (the bulk):
  - Consume `ListingStockChanged`: a new `case` decodes the payload and applies the stock as an atomic,
    version-guarded partial update. It never creates a document, never resurrects a deleted one, and a stale or
    redelivered event is a no-op (AD2).
  - Stock carries its **own** monotonic guard (`stock_version`, from the envelope `occurred_at`), separate from
    the document `version`, so a stock event is neither blocked by a newer price/status event nor able to
    shadow an older base-info event (design D2).
  - Index fields `stock` (`integer`) and `stock_version` (`long`), added to the **current** mapping with an
    idempotent put-mapping at startup, and included in the create-index mapping for fresh clusters.
  - Full `ListingChanged` upserts stop clobbering stock: they carry forward an existing stock, and only seed
    `stock` from `Listing.stock` when the document has none (design D3).
  - `filters["in_stock"]="true"` restricts a search to listings with `stock > 0`.
  - A malformed stock event (empty listing id, negative stock, missing `occurred_at`) fails the record into the
    existing AD1 retry-then-DLQ path; it never advances the offset silently.
- **platform-core**: one additive, optional field, `optional int32 stock = 3;` on `SearchHit`
  (`platform/search/v1/search.proto`). `buf breaking` passes. Existing callers ignore it.
- **team-gateway, team-frontend, team-ai**: re-vendor and regenerate the search proto only; the gateway
  forwards the message unchanged (Rule 2). No UI behaviour change is required by this change.
- **platform-e2e**: fix `SearchService.indexed_stock` to read `hits[].listing_id` / `hits[].stock` and add the
  scenarios below.
- Composition with `add-hybrid-retrieval-platform` (in flight, 0/39): this change lands first against the
  current single index. The v2 mapping must include `stock` and `stock_version`; replay repopulates them (design D5).
- No **BREAKING** behaviour: the response gains an optional field, the mapping gains two fields, and a previously
  ignored event now has an effect.

## Capabilities

### New Capabilities
- `search-stock-read-model`: the search read-model holds each listing's current stock, derived from
  `ListingStockChanged`, kept correct under redelivery, reordering and failure, optionally exposed per hit, and
  usable as an `in_stock` filter.

### Modified Capabilities
<!-- No search capability exists in openspec/specs/ yet (search-retrieval is still an in-flight change in
add-hybrid-retrieval-platform and is not in main specs). Nothing to modify. -->

## Impact

- Repos: `team-search` (consumer, index, handler), `platform-core` (one optional proto field),
  `team-gateway` / `team-frontend` / `team-ai` (vendored proto regeneration only), `platform-e2e` +
  `team-search/FEATURES.yaml`.
- Contract: one additive `optional` field; `buf lint` and `buf breaking` must pass. No new topic, no new event
  (the event and its proto already exist; Rule 4 respected, contract not forked).
- Data: OpenSearch `listings` gains `stock` and `stock_version` via put-mapping (additive, no reindex). Existing
  documents have unknown stock until their next stock event or a replay (see design Migration Plan and runbook).
- Runtime: team-search indexer applies one more event type; partial updates add one scripted update per stock
  event. Kafka offset discipline (AD1) and the version guard (AD2) are unchanged and reused.
- Architecture rules: Rule 3 (team-search keeps its own index; it reads no domain DB, the event is the only
  source), Rule 4 (contract change only in platform-core), Rule 5 (state-change event on Kafka, already so).
- Ordering with `add-hybrid-retrieval-platform`: independent, this change first; that change's task 3.2 must
  add the two fields to the v2 mapping.

## Non-goals

- No hybrid/semantic retrieval, alias-addressed indexes, RRF or rerank (that is `add-hybrid-retrieval-platform`).
- No ranking or sorting by stock; `stock` is a filter and a displayed value only.
- No inventory semantics: no reservation, commit, release or oversell logic; the read-model mirrors the number
  the domain publishes, and the domain stays the source of truth for purchasing decisions.
- No per-variant stock in the index (the event's `variants` are ignored; the index mirrors the event's base `stock`; a variant-sum definition is a future domain-side decision).
- No new Kafka topic, no change to `team-domain`'s publisher, and no change to gateway routing or auth scopes.
- No dedicated backfill job; replay of `listing.events` is the rebuild path.
