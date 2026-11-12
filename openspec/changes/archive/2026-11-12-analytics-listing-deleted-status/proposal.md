# Proposal: analytics-listing-deleted-status

initiative: discovery-followups

## Why

When team-domain hard-deletes a listing it emits `ListingChanged(CHANGE_TYPE_DELETED)`. team-analytics keeps the row (historical
tracking events stay attributable to the seller) but blanks its `status`
(`team-analytics/internal/events/listing.go:43-46`), so `listing_sellers.parquet`, `item_attributes@v2` and the recsys payload
cannot tell a deleted listing from one whose status was never known. recsys leaves `visible` absent for an unknown status, which
is "not eligible" only by the absent-means-ineligible rule. discovery-seo left this open (plans/discovery-seo/LOCK.md "Open
issues"). The locked decision (plans/discovery-followups/00-spec.md, 2026-11-12) is an explicit `DELETED` status written by
team-analytics; team-domain events do not change.

## What changes

- `uses`: `discovery-followups-contract` adds `LISTING_STATUS_DELETED = 7` to `ListingStatus` (not redefined here);
  `listing.events/ListingChanged` is consumed unchanged.
- team-analytics: the decoder `ListingSellerFromEnvelope` writes `status = LISTING_STATUS_DELETED` when
  `change_type = CHANGE_TYPE_DELETED` (replacing the blanking), in the live consumer and in `listing-replay` (same decoder). The row is
  still kept. DELETED is terminal: the duckdb upsert refuses any later non-DELETED event for a DELETED row. Re-vendor the proto under `team-analytics/proto` and regenerate so the enum has value 7.
- platform-featurestore: `LISTING_STATUSES` (`featurestore/inputs.py`) gains `LISTING_STATUS_DELETED`, so the value reaches
  `item_attributes@v2.status` instead of being mapped to NULL. The v2 SQL, definition and lock hash are unchanged (no version bump).
- platform-recsys: `KNOWN_STATUSES` (`recsys/load/attributes.py`) gains `LISTING_STATUS_DELETED`; `visible = status == PUBLISHED`
  stays, so a deleted listing gets `visible=false` (present) instead of an absent key.
- platform-e2e: scenarios for export, replay and recsys payload; the recsys driver helper
  `tests/e2e/flows/item_attributes_support.py` may be shared with recs-in-stock-filter (serialised by initiative-build).
- Delta specs MODIFY `item-attributes` (export requirement, replay requirement, v2 requirement) and
  `recsys-candidate-attributes` (item payload requirement). The old scenario "A deleted listing keeps its row with a null
  status" is replaced.

Out of scope: team-domain, search, ai-listing-index (they do not read the analytics status), restoring deleted listings,
the deploy runbook for running `listing-replay`.

## NFR

- No new RPC, event, table or column. Export freshness unchanged: a DELETED status is visible in the parquet within
  2 x `PARQUET_EXPORT_INTERVAL_SECONDS` of consumption.
- Decoder and upsert stay O(1) per event; replay throughput unchanged (non-gating target from the prior change: 100,000 events
  under 5 minutes).
- Backward compatibility: a consumer of the parquet that does not know the string `LISTING_STATUS_DELETED` sees a non-PUBLISHED
  status, so it stays invisible by the existing `== PUBLISHED` rule. Deploy order: platform-core proto, team-analytics, then
  featurestore and recsys (an older featurestore maps the new value to NULL, which recsys already treats as not eligible).
- Replay needs the delete event to still be on `listing.events`; a listing whose events expired keeps its null status (known gap, scenario given).

## Data classification

| field | class | purpose | consent basis | retention |
|---|---|---|---|---|
| `listing_id`, `seller_id`, `category_id`, `price`, `status` | non-personal business data (a seller id is a pseudonymous account id, no contact data) | seller funnel attribution, item attributes for recommendations | not applicable (no personal data of buyers; seller id as in the existing export) | unchanged: row kept for the life of the warehouse; no new field |

`status` adds one enum value. No log line may carry a listing or seller id (existing rule, kept).

## Threat model

- Assets: integrity of `status` in the warehouse and downstream visibility flag.
- Abuse: (1) a stale, replayed, equal-timestamp or later non-DELETED event resurrecting a deleted listing as visible: DELETED is terminal in team-analytics (human decision 2026-11-12: team-domain hard-deletes and ids are never reused), covered by the scenarios "An older event does not undo DELETED", "Equal timestamp: a non-DELETED event consumed after DELETED does not undo it" and "A newer non-DELETED event does not undo DELETED"; (2) a forged `change_type` cannot occur without write access to `listing.events`
  (service-to-service topic, unchanged trust); (3) the replay command rewriting rows, run by an operator with warehouse access only.
- Authz: no new endpoint; analytics query RPCs and their authz are untouched. No auth, payment or PII risk.

## Open points

- DELETED is terminal (human decision): a listing re-created under a deleted id would stay DELETED; ids are never reused, so this is accepted.
