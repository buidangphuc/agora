## Context

See proposal.md (Why) for the four defects. The constraints that shape the approach, all verified in agora:

- **One index, two retrieval legs.** `internal/retrieval/engine.go` runs `Index.Search` (BM25) and `Index.SearchVector`
  (k-NN, `knn.filter`) against the same `listings` index, fuses with RRF and fails open to either leg. Both legs build
  their filters with `buildFilterClauses` (which also adds the published-only default), so that builder is the one
  place a filter can be enforced for every mode, total and facet. There is no alias or v2 index.
- **Two version mechanisms today.** `Upsert` uses OpenSearch external versioning (`_version` = `occurred_at` ns);
  `PartialUpdate` uses a painless guard on the `_source.version` field. The update API bumps the internal `_version`
  by one, so the two are only loosely aligned.
- **Per-listing order.** Every listing event (including `ListingStockChanged`) goes through `team-domain`'s single
  outbox, is keyed by `listing_id` and lands on one partition; the indexer handles a partition serially and commits
  only after success or DLQ (AD1). Out-of-order delivery for one listing therefore comes from replays, redelivery
  after a crash, or a foreign publisher, which is what the e2e black-box cases simulate.
- **Stock semantics.** `ListingStockChanged.stock` is the listing's base stock after the change, read inside the
  stock transaction; `occurred_at` is stamped in that transaction, and the row lock on `listings` makes two stock
  events for one listing carry increasing `occurred_at`. `UpdateListing` writes `stock` from the request (a seller
  edit), so a newer `ListingChanged` carries authoritative stock.
- **No rating anywhere.** `Listing` has no rating field, the indexer consumes only `listing.events`, and `toDoc` never
  sets `Rating`.
- **Frontend rating path.** `team-frontend` parses `?rating=` (`url.ts`, `/^[1-5]$/`), sends it as `minRating`
  (`data.ts` → `lib/gateway/search.ts`), renders the ratings group whenever `facets.ratings` is non-empty
  (`FilterSidebar.tsx`, inline card and mobile drawer), shows a rating chip (`ActiveFilters.tsx`), carries `rating` in
  every href and the filter form's hidden fields, and renders an error state when the search call fails.
- **Edge.** `team-gateway` forwards `SearchService` messages using its generated types (`internal/edge/search.go`),
  and Connect JSON drops unknown fields, so a new response field only reaches clients after the gateway re-vendors.

## Goals / Non-Goals

**Goals:** one write guard for every document write; stock ordered independently of the other fields; tombstones that
compose with that guard; filters and sorts that hold in every search mode; no proto change beyond `SearchHit.stock`.

**Non-Goals:** a v2 index or alias flip; changing `team-domain`'s events; metrics dashboards for the new paths (log
lines only); per-variant stock (D13); any frontend change beyond the rating control (D12).

## Decisions

### D1. Every document write is a scripted update guarded on `_source.version`

`Upsert` becomes an update with `scripted_upsert: true` whose script (a) creates the document when absent, (b) is a
`noop` when the stored `version` is at or above the incoming one, (c) handles tombstones (D5), and otherwise writes
the base fields while carrying `stock`/`stock_version` (D2) and `created_at` (D7) forward. `PartialUpdate` keeps its
script with the tombstone check added. One atomic request per event, no read-before-write.

*Alternatives:* keep external versioning and do GET-then-index to carry fields forward: two round trips, and the
external `_version` and the script guard keep drifting apart. Keep external versioning and store stock in a separate
index: needs a join at query time for the `in_stock` filter on both legs.

### D2. Stock has its own guard, `stock_version`

`stock_version` is the `occurred_at` (ns) of the event that last set `stock`. A `ListingStockChanged` is a scripted
update *without* upsert (a missing document gives 404 and is acknowledged: "never creates"); the script is a `noop`
on a tombstone or when `stock_version >= incoming`, else sets `stock` and `stock_version` and leaves `version` alone.
A `ListingChanged` sets `stock` from `Listing.stock` only if its version is newer than `stock_version`, otherwise the
stored stock survives. Separate guards mean a stock event is never blocked by a newer price or title event and never
shadows an older base-info event.

*Alternative (the carried-over design D3):* seed `stock` from `ListingChanged` only when the document has none. Wrong
for agora: a seller's `UpdateListing` changes stock and arrives only as `ListingChanged`, so it would never reach
search.

Validation in the consumer: empty `listing_id`, `stock < 0` or missing `occurred_at` returns an error, so AD1 retries
and parks the record. Variants in the event are ignored (proposal Non-goals).

### D3. `SearchHit.stock` is `optional int32 = 3`, carried through fusion

The index decodes `stock` from `_source` into `index.Hit` (pointer, so "absent" survives). `retrieval.Candidate`
carries it through `RRF`, rerank and pagination (fusion keys by listing id; the stock comes from whichever leg
returned the hit). The handler maps it onto `SearchHit.stock` for `SearchListings` and `RunSavedSearch`. Field 3 is
free in agora's `search.proto` (1 `listing_id`, 2 `score`).

### D4. `in_stock` is a filter-map key, enforced in the shared filter builder

`buildFilterClauses` consumes the `in_stock` key (it must never fall through to a raw `term` on a non-existent field)
and adds `range stock > 0`; documents without `stock` do not match a range. Because both legs and the facet
aggregation use that builder, every mode, total and facet agrees. The handler's `effectiveFilters` (shared by
`SearchListings`, `SaveSearch` and `RunSavedSearch`) rejects any value other than `"true"` with `INVALID_ARGUMENT`.

*Alternative:* `bool in_stock = 10` on `SearchListingsRequest`. Rejected: saved searches persist only `filters_json`,
so a request field would not reach `RunSavedSearch` without a second contract change, and `filters` is already the
structured-filter channel.

### D5. Tombstones are versioned documents

All three delete paths write, through the D1 script, `{id, status: "deleted", version: occurred_at, tombstoned_at:
now}` and drop every other field. Rules in the scripts:

- full upsert: `noop` when the incoming version is `<=` the tombstone's or the incoming version is 0 (no
  `occurred_at`); a newer full upsert replaces the tombstone (only the domain can emit one after a delete, e.g. a
  re-approved listing);
- partial update and stock update: always `noop` on a tombstone;
- a delete over an existing tombstone keeps the newer version.

`status: "deleted"` is excluded by the published-only default, the owner draft view uses `status=draft`, and
`effectiveFilters` already rejects any other status value, so no read path can return a tombstone. `Suggest` filters
on `status=published`.

*Alternative:* rely on OpenSearch's `index.gc_deletes`. Rejected: it is a short, cluster-wide window and not a
contract.

### D6. Purge runs in the indexer, by processing time

`cmd/indexer` starts a loop that every `TOMBSTONE_PURGE_INTERVAL` (default `1h`) runs `delete_by_query` for
`status=deleted AND tombstoned_at < now - TOMBSTONE_TTL` (default `336h`), logging the count. `tombstoned_at` is the
indexer's wall clock when it applied the delete, so the retention protects against late redelivery measured from
when the read-model learned of the delete, and a replay of an old delete re-arms it. Config follows the
`RESERVATION_TTL` pattern: unparsable or non-positive falls back to the default with a WARN; keys go into
`.env.example` (drift gate). The query server never purges.

### D7. `created_at` is set-if-earlier, outside the base-field guard

The CREATED branch passes the envelope `occurred_at` as `created_at`. Both the upsert script and a dedicated branch for
a stale CREATED (its base fields are rejected by the guard) set `created_at` when it is absent or later than the
incoming value, unless the document is a tombstone. UPDATED events never set it; the D1 script carries it forward.
`SORT_BY_NEWEST` becomes `[{created_at: {order: desc, missing: _last}}, {id: asc}]` in both legs (`id` is the keyword
field; sorting on `_id` is not used).

### D8. A key sort bypasses fusion

RRF ranks by relevance, so it destroys any key order. When `sort_by` is `NEWEST`, `PRICE_ASC` or `PRICE_DESC`, the
engine serves a `HYBRID` (or defaulted-to-hybrid) request from the lexical leg with that sort and the requested page;
`SEMANTIC` keeps the k-NN query with the sort applied to its k candidates. `RELEVANCE`/unspecified is unchanged.

*Alternative:* fuse, then re-sort the fused page by a key read from `_source`. More code and the fused candidate pool
is capped at the fusion window, so deep key-sorted pages would differ from the lexical ones anyway.

### D9. Rating honesty

The handler rejects `min_rating != 0` with `INVALID_ARGUMENT` before calling the engine or index (so both legs are
covered), `facetAggs` drops the `ratings` aggregation and `parseFacets` returns an empty `Ratings` slice, and
`ListingDoc.Rating` is no longer written. The `rating` mapping stays so a future rating source needs no mapping
change; that source would need its own change to lift the rejection. The frontend side is D12.

### D10. Mapping evolution

`EnsureIndex` keeps creating the index with the full mapping (now including `stock: integer`, `stock_version: long`,
`created_at: date` (epoch millis), `tombstoned_at: date`) and, when the index exists, issues an idempotent
`PUT _mapping` for the four fields. Both processes call it at boot; a concurrent identical put is harmless.

### D11. E2E technique

- Positive assertions poll through the gateway with a bounded deadline (30 s for indexing, 60 s for DLQ).
- Out-of-order cases publish a binary `EventEnvelope` to `listing.events` keyed by `listing_id` with a chosen
  `occurred_at`, using a small protobuf field encoder next to the existing decoder in
  `tests/e2e/flows/tracking_flow.py`. The "redelivered create" case re-publishes the original record's bytes read
  with `oic_inv_events_flow`-style scanning.
- Negative assertions ("still absent", "still 8") first wait until the `team-search-indexer` consumer group's
  committed offset on that record's partition is beyond the published record's offset, then assert once. No sleeps.
- The purge scenario uses a compose overlay (`TOMBSTONE_TTL=30s`, `TOMBSTONE_PURGE_INTERVAL=2s`) and reads OpenSearch
  (`GET listings/_doc/<id>`) to observe the tombstone and its removal, the store-state check README §5 asks for. The
  other tombstone scenarios publish their stale event right after the delete and finish well inside 30 s; they run in
  the serial lane because of the short TTL.
- The `frontend-search-rating` scenarios drive `team-frontend` :3000 with Playwright page objects (search results page,
  filter sidebar); they assert the rendered DOM and link `href`s, since server-side gateway calls cannot be
  intercepted from the browser. That no `minRating` is sent is pinned by `team-frontend` unit tests (D12).

### D12. The frontend drops rating entirely, and ignores it in URLs

`parseSearchParams` no longer reads `rating` and `SearchState` loses the field, so an old `?rating=4` link parses to
the same state as the link without it: no `minRating` is sent, hrefs, the hidden form fields and the active-filter
count never carry it, and no chip renders. `FilterSidebar` drops the ratings group in both the inline card and the
drawer rather than relying on an empty facet list, so the UI does not depend on search's facet output.
`searchListings` loses its `minRating` option so no caller can reintroduce it.

*Alternatives:* keep the group behind `facets.ratings.length > 0` (already empty after D9): leaves dead code and the
`?rating=` → error path. Redirect `?rating=` to the clean URL: an extra round trip for no benefit; ignoring is
enough because the page builds clean links from then on.

### D13. Stock is base-listing stock only (decided)

`stock` in the index and on `SearchHit` is the listing's base `stock` exactly as `team-domain` publishes it in
`Listing.stock` and `ListingStockChanged.stock`; `variants` in either event are ignored, and `in_stock` tests that base
value. Decided by the human on this change: no agora listing flow, seed or UI uses variants today. A variant-aware
definition (for example the sum of variant stock, with a version per variant) would be a later change, together with
a domain-side definition of base stock for listings that have variants.

## Risks / Trade-offs

- [A stock event that arrives before its listing's CREATED is dropped, and the CREATED then seeds an older stock] →
  impossible with the single outbox per listing; a replay restores order. Accepted rather than creating stub docs.
- [An event older than the tombstone but delivered after the purge resurrects the listing] → default retention 14 days
  versus a replay that processes events in order; documented in the README runbook.
- [The scripted upsert costs more CPU than a plain index] → one request per event either way; the indexer is not on
  the query path.
- [Existing documents have no stock or creation time until replay] → `SearchHit.stock` is absent (not 0), `in_stock`
  excludes them, newest sorts them last. Runbook: reset the indexer group to the earliest offset after deploy.
- [An old frontend build sends `min_rating` from `?rating=` and gets `INVALID_ARGUMENT`] → deploy the D12 frontend
  before or with `team-search` (Migration Plan step 2).
- [A listing with variants and base stock 0 is hidden by `in_stock`] → accepted with D13; no such listings exist today.
- [A key sort in hybrid mode loses semantic recall] → intended: a key-ordered list has no relevance order to fuse.

## Migration Plan

1. Merge the proto field in `platform-core`; re-vendor `team-search` and `team-gateway` (others follow, no behaviour).
2. Deploy `team-frontend` (D12) first or together with `team-search`: it no longer sends `minRating`, which the old
   `team-search` accepts too, so it is safe in either order with the old search.
3. Deploy `team-search` server and indexer together (both run `EnsureIndex`; the put-mapping is additive).
4. Reset the `team-search-indexer` consumer group to the earliest offset of `listing.events` to backfill stock, creation
   time and tombstones (`rpk group seek team-search-indexer --to start`); the D1 guard makes the replay idempotent.
5. Rollback: redeploy the previous image. The extra fields are ignored by the old code; tombstone documents have
   `status=deleted` and stay hidden by the published-only default; old code's plain index would overwrite them, which
   is today's behaviour.
