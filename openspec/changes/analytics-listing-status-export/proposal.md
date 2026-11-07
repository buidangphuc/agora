# Proposal: analytics-listing-status-export

initiative: discovery-seo

## Why

`platform-recsys-candidate-attributes` (C10) must write a `visible` flag into recommendation payloads, and `visible` is
`status == LISTING_STATUS_PUBLISHED` (C13). The only source of listing attributes for the featurestore is
`listing_sellers.parquet` (`platform-featurestore/featurestore/inputs.py:22,140-155`), and it has no status column.
Without it a banned, held, draft or pending listing cannot be told apart from a published one. C13 (human, 2026-11-07)
adds a `status` column to that export; this slice does it. Nothing else about the export changes.

Verified state of team-analytics (read from code, not assumed):

- It already consumes `listing.events` / `platform.listing.v1.ListingChanged` (envelope type discriminator
  `ListingChangedEventType`, `team-analytics/internal/events/listing.go:14-17`). The decoder
  `ListingSellerFromEnvelope` (`internal/events/listing.go:19-49`) reads `Listing.id`, `seller_id`, `category_id`, `price`
  and the envelope `occurred_at`. It does NOT read `Listing.status` or `ChangeType` today. The consumer is wired at
  `internal/app/consumer.go:118-129` (own group, `AtStart`, DuckDB only, topic `KAFKA_LISTING_TOPIC`).
- The record is `warehouse.ListingSellerRecord` (`internal/warehouse/warehouse.go:222-232`), the table is created and
  column-migrated in `internal/warehouse/duckdb/duckdb.go:115-131` (`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`), and the
  upsert is `upsertListingSellerSQL` / `UpsertListingSellers` (`duckdb.go:520-562`; an event older than the stored row
  never overwrites it, `WHERE excluded.updated_at >= <table>.updated_at`).
- The export is `internal/export/export.go`: `Targets` (line 33-40) lists `listing_sellers` and `Once` (line 78-97) copies
  the whole table with `SELECT *`-style `ExportRelation`, so a new table column reaches the parquet with no export code
  change.
- Deleted listings: `ListingSellerFromEnvelope` ignores `ChangeType`, and the writer never deletes (comment at
  `warehouse.go:239-242`, README line 40, `openspec/specs/item-attributes/spec.md` "A deleted listing SHALL keep its
  row"). So today a deleted listing STAYS in the export. This is kept (historical tracking events must remain
  attributable to the seller, used by `GetSellerFunnel`).
- Enum names: `ListingStatus` is defined in `platform-core/packages/proto/platform/listing/v1/listing.proto:321-329`:
  `LISTING_STATUS_UNSPECIFIED`, `_DRAFT`, `_PUBLISHED`, `_REJECTED`, `_PENDING_REVIEW`, `_BANNED`, `_HELD`.
  **There is no `LISTING_STATUS_DELETED` in the proto.** Deletion is only `ChangeType.CHANGE_TYPE_DELETED`
  (`listing.proto:402-407`), and a DELETED event still carries a `Listing` whose `status` is whatever it was last.
- No history of status exists in `listing_sellers` (the column does not exist), so existing rows have nothing to backfill
  from the table itself.

## What changes

- `ListingSellerRecord` gains `Status string`; `ListingSellerFromEnvelope` sets it to the enum name of `Listing.status`
  (`listingv1.ListingStatus.String()`, for example `LISTING_STATUS_PUBLISHED`), except:
  - `ChangeType == CHANGE_TYPE_DELETED`: status is NULL (see Decision D1);
  - `LISTING_STATUS_UNSPECIFIED` (0): status is NULL.
- DuckDB: idempotent column migration `status VARCHAR` on `listing_sellers` (same pattern as `category_id` / `price`);
  `upsertListingSellerSQL` writes `status` with the same newest-event-wins guard.
- `listing_sellers.parquet` columns become `listing_id, seller_id, updated_at, category_id, price, status`.
- Backfill (C15): existing rows get `status` NULL after the migration. A new one-off command binary
  `team-analytics/cmd/listing-replay` (modelled on `cmd/ga4import`, a command, not an enable flag) re-reads
  `KAFKA_LISTING_TOPIC` from the earliest offset on its OWN consumer group (`KAFKA_LISTING_REPLAY_GROUP`, default
  `team-analytics-listing-replay`, distinct from the live `KAFKA_LISTING_CONSUMER_GROUP` at `internal/config/config.go:52`),
  upserts through the same decoder and `UpsertListingSellers`, and exits 0 when it reaches the end offsets captured at
  start. It runs once at deploy. DuckDB takes an exclusive file lock (`internal/export/export.go` package comment), so
  the live consumer process is stopped while it runs (runbook step in the README). Replay is idempotent and order-safe
  through the `updated_at` guard.
- README rows for the table, the export and the consumer are updated; FEATURES.yaml is left to the e2e-author.

## Decisions

- D1 Deleted listing: the row stays in the export (existing behaviour, `GetSellerFunnel` depends on it) and its
  exported `status` is NULL, not `LISTING_STATUS_DELETED`, because that enum value does not exist and adding one is a
  proto change outside this slice. A DELETED event's `Listing.status` is not trusted: a listing deleted while
  PUBLISHED would otherwise stay exported as PUBLISHED, so a consumer would show a deleted listing. NULL is read by the
  consumer as "not visible" (it fails closed on a missing status; `recsys-candidate-attributes` and C13).
  This deviates from the literal "latest `Listing.status`" in C13 only for DELETED events. Confirmed by the human in
  C15. A `LISTING_STATUS_DELETED` enum is a possible later proto follow-up, out of this initiative.
- D4 Equal `updated_at`: the later-consumed event wins (the guard is `>=`), which is also what makes replay refill a row.
- D2 `status` is a string holding the enum NAME, not the number, so a reorder or addition in the proto cannot change
  meaning; the consumer compares to `LISTING_STATUS_PUBLISHED`.
- D3 No second pass or view: the status is stored beside the other attributes in the same row, so the exported row is
  one consistent snapshot of one event.

## NFR

- Export cost: one extra VARCHAR column; export of 100,000 listing rows SHALL take under 60 s (measured, non-gating).
- Freshness: a status change SHALL be visible in `listing_sellers.parquet` within 2 x `PARQUET_EXPORT_INTERVAL_SECONDS`
  of its consumption (setting `internal/config/config.go:75`, default 0 = export off, root compose sets 300, README
  line 73, so 600 s there); the export is atomic tmp-then-rename.
- Replay: the one-off replay of 100,000 listing events SHALL finish in under 5 minutes (measured, non-gating).
- Availability: the column migration runs at startup and is idempotent; a failed migration stops startup with an error
  (like the existing columns). The parquet is still replaced atomically (tmp then rename), so readers never see a file
  without the column halfway.
- Compatibility: the new column is appended last; the consumer reads it as optional, so deploy order is exporter first.
- Limits: `status` is at most the longest enum name (29 characters); values outside the proto enum are never written.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `status` (listing moderation/publication state) | Business data about a listing, not personal data on its own | Lets the recommender hide unpublished listings | Not personal data; no consent required | Same as the row: kept while the listing mapping is kept (never deleted today) |
| `seller_id` (already exported) | Pseudonymous identifier of a seller | Seller attribution | Unchanged by this slice | Unchanged |

The status can reveal that a listing was banned or held, which is operationally sensitive; it is exported only to the
analytics volume that is mounted read-only by platform-featurestore and platform-recsys (existing). It is never logged
with a listing or seller id (repo rule: no raw ids in logs).

## Threat model

- Assets: the integrity of `status` (a wrongly PUBLISHED value makes a banned or deleted listing recommendable); the
  confidentiality of moderation state.
- Abuse cases: (1) out-of-order or replayed event regressing a newer status, mitigated by the `>=` guard on
  `updated_at`; (2) forged status in an event, out of scope (topic is internal, authz at the broker); (3) a deleted
  listing remaining PUBLISHED, mitigated by D1; (4) an unknown or future enum number written as a number or as
  PUBLISHED, mitigated by writing only known enum names and NULL otherwise.
- Authz rule: no new RPC and no new endpoint. The file is readable only through the existing read-only volume mount;
  the consumer and the export remain internal. No permission scenario applies.
