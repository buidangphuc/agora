# Proposal: recsys-candidate-attributes

initiative: discovery-seo

## Why

team-ai's recommendation read path knows three things about a candidate: `listing_id`, `score`, `in_stock`
(`team-ai/app/modules/business/recommendation/schemas.py:11-16`). The Qdrant payload written by the loader is
`{listing_id|user_key, model_version, updated_at}` (`platform-recsys/recsys/load/qdrant.py:105-117`) and the Redis
entries are `{listing_id, score}` (`platform-recsys/recsys/load/redis_cache.py:_encode`). So `deduplicate_sellers`,
`category_popular` and the "hidden or unpublished listings never appear" guarantee of `recsys-cart-cross-sell`
cannot work. Decision C10 (human, 2026-11-07): the producer writes the attributes into the retrieval payloads; there is
no request-time enrichment call.

Verified state of the producer side:

- `item_attributes@v1` ALREADY carries `seller_id`, `category_id`, `price`
  (`platform-featurestore/registry/features.yaml:47-54`, `registry/sql/item_attributes.sql:3-6`). Only the listing
  status is missing. The initiative text says `seller_id` is absent; it is not.
- The view reads the point-in-time table `listings` built from `listing_sellers.parquet`
  (`featurestore/inputs.py:22,140-155`), exported by team-analytics (`listing_sellers` columns `listing_id, seller_id,
  updated_at, category_id, price`, see `openspec/specs/item-attributes/spec.md`). The export has no status column.
- A view definition cannot change without a version bump (`registry/features.lock`, `registry/features.yaml:1-2`), so
  the status is added as `item_attributes@v2`; v1 stays untouched for its current readers
  (`recsys/two_tower/stage.py:37`, `recsys/ranker/*`).
- `in_stock` is written by nobody: not by the loader, not by any featurestore view. team-ai defaults it to true
  (`adapters/qdrant.py:107`, `adapters/redis_cache.py:17`). Decided (C14): the loader does not write `in_stock` in this
  initiative, so `in_stock_only` stays a documented no-op.
- The recsys loader receives only ids and vectors (`publish.py:publish_generation`); it has no listing metadata today.

## What changes

depends_on: [analytics-listing-status-export] (owner team-analytics, C13), which adds the `status` column
(`ListingStatus` enum name) to `listing_sellers.parquet`. This slice reads it as an OPTIONAL column so deploy order is
safe: without the column `status` is null, `visible` is absent and the consumer fails closed.

## Decisions

- C13: `status` comes from `analytics-listing-status-export`; `visible = (status == LISTING_STATUS_PUBLISHED)`, every
  other status is `visible=false`.
- C14: no `in_stock` written; `in_stock_only` stays a documented no-op.
- Approved design: `item_attributes@v2`; settings `ITEM_ATTRIBUTES_V2_DIR` / `ITEM_ATTRIBUTES_V2_PATH`; command
  `python -m recsys backfill-attributes` patching the serving and previous generations; a missing v2 snapshot logs
  `attributes=missing` and writes the old payload shape without aborting.
- C15: `analytics-listing-status-export` provides a one-off replay that fills `status` for existing listings.
- C16: a v2 snapshot whose `as_of` is older than 24 h makes the loader log a warning with `as_of` and still publish.
  `visible` may lag a moderation change by at most one cycle of export, featurestore run, and publish or backfill.
- Deploy order: analytics replay (C15), then featurestore v2, then recsys publish or backfill, then the consumer
  (`recsys-cart-cross-sell`). Any generation without `visible` yields no eligible candidates for the consumer.
- Merge order: unit tracks (analytics, featurestore, recsys), then the consumer, then the e2e track.
- Unknown status: `LISTING_STATUS_UNSPECIFIED` or an unrecognised name counts as unknown (`status` null, `visible` absent).
- Backfill also patches the legacy unscoped `recs:v1:{user,item,popular}` keys; a key with no TTL stays without TTL; a
  key that expired between scan and rewrite is skipped.
- Adding v2 flips `fs:item_attributes:current` to `2`; v1 online keys stay readable until their TTL.

- platform-featurestore: new view `item_attributes@v2` (per listing: `seller_id`, `category_id`, `price`, `status`),
  `status` read from an OPTIONAL `status` column of `listing_sellers.parquet`; `item_attributes@v1` unchanged.
- platform-recsys: `publish_generation` reads the latest `item_attributes@v2` snapshot (new path setting
  `ITEM_ATTRIBUTES_V2_DIR` / `ITEM_ATTRIBUTES_V2_PATH`, same pattern as `ITEM_ATTRIBUTES_DIR`; a directory, not an enable
  flag) and writes `seller_id`, `category_id`, `visible` into every item point payload (ALS item collection and
  two-tower item collection) and into every entry of every Redis user, item and popular key (generation-scoped and
  legacy). `visible` is true only when `status == LISTING_STATUS_PUBLISHED`.
- Missing attribute rule: an attribute that is unknown is ABSENT from the payload or entry (never `""`, never a
  substituted constant). `visible` is written for every listing that has a known status (true or false) and absent
  otherwise; the consumer treats absent `visible` as not visible (fail closed).
- Rollout: the next publish writes the fields (generations are written fresh). A new command
  `python -m recsys backfill-attributes` patches the serving and previous generation in place so a deployment does not
  wait for the next training run.
- No new enable flag; no import-guarded fallbacks.

## NFR

- Publish: attribute lookup is one in-memory dict built from one snapshot read; publish duration grows by under 10% on
  the fixture of 10,000 items (non-gating measurement).
- Redis entry size: at most 3 added keys per entry; entries stay below 64 KiB for `TOP_N` entries.
- Measured, non-gating: publish duration delta and backfill throughput. Unit-gated: at most 3 added keys per entry, Qdrant batch size 256, entries under 64 KiB at `TOP_N`.
- Backfill: patches Qdrant with `set_payload` in batches of 256 and Redis keys without changing their TTL; running it
  twice yields identical state (idempotent). Serving reads are never blocked: vectors and scores are untouched.
- Availability: a missing or unreadable v2 snapshot never aborts a publish; entries are written without the three keys
  and the publish log says so (`attributes=missing`).
- Limits: `seller_id` and `category_id` are written as received, trimmed, at most 128 characters; longer values are
  treated as unknown.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `seller_id` | pseudonymous id of a business actor (Principal id) | one listing per seller dedupe in cross-sell | contract (operating the marketplace) | as the generation: dropped by generation retention (serving + previous) and Redis TTL `CACHE_TTL_SECONDS` |
| `category_id` | non-personal catalogue data | category-based popular fallback | not personal | same |
| `visible` | non-personal derived flag | never recommend unpublished listings | not personal | same |
| `status` (featurestore only) | non-personal | source of `visible` | not personal | offline snapshot: earlier runs are kept, never pruned (`platform-featurestore/README.md:44`); online `fs:item_attributes:v2:*` keys expire after `FEATURESTORE_ONLINE_TTL_SECONDS`, default 172800 s = 48 h (`featurestore/settings.py:47`) |

No buyer data is added. `seller_id` is never logged: the loader logs counts only (repo rule: no raw ids in logs).

## Threat model

- Assets: the guarantee that a banned, held, draft, rejected or pending listing is never recommended; seller ids in the
  cache.
- Abuse cases: (1) a moderated listing keeps `visible=true` after a ban, so it is recommended: mitigated by fail-closed
  absent handling and by regenerating `visible` from the status on every publish and backfill; (2) a stale payload with
  no `visible` key makes everything vanish: accepted as the safe failure, handled by backfill and deploy order (producer
  before consumer); (3) an attacker-controlled listing field injected into the payload: only ids and a boolean derived
  from an enum are written, no free text.
- Staleness: `visible` may lag a moderation change by at most one cycle of export, featurestore run, and publish or backfill; a v2 snapshot older than 24 h is warned about (`as_of` logged) but still published (C16).
- Authz rule: no request path is added. Writes happen only from the batch job that already holds Redis and Qdrant
  credentials; the new command runs under the same credentials. Reads stay with team-ai. Authz does not apply to this slice beyond the existing store credentials (a backfill without them exits non-zero and writes nothing).
