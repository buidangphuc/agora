## Why

initiative: monetization

Seller plan caps are stored (`GetEntitlements` returns `max_listings` and `featured_listings`) but nothing enforces
them: `CreateListing` (`team-domain/internal/rpc/listing.go`) has no cap and the storefront accepts any number of
featured ids. A FREE seller (max_listings 10) can create unlimited listings, so the paid plans buy nothing. This slice
makes team-domain enforce both caps by asking team-promotion for the seller's entitlements.

## What Changes

- New required config `UPSTREAM_PROMOTION_ADDR` (team-domain, no `*_ENABLED` flag, startup fails when empty) and a gRPC
  client to `SubscriptionService.GetEntitlements`, behind a small port in `internal/listing` so unit tests use a fake.
  The call is made with the service principal and an explicit `seller_id`; timeout and retry follow the existing
  upstream client defaults.
- `CreateListing`: before inserting, read `max_listings`, count ALL of the seller's listings (any status) in the
  team-domain store, and return `RESOURCE_EXHAUSTED` with `errdetails.ErrorInfo{Reason: "PLAN_LISTING_LIMIT"}` when
  count >= cap. `-1` means unlimited. The count and the insert run in one transaction under a per-seller advisory
  lock so concurrent creates cannot overshoot the cap.
- `UpsertStorefront`: `len(featured_listing_ids)` is capped by `featured_listings` from the plan
  (`RESOURCE_EXHAUSTED`, reason `PLAN_FEATURED_LIMIT`); this replaces any fixed limit of 6 from the storefront cluster.
  Ids already stored may stay (a lowered cap never rewrites a storefront); only a write that exceeds the cap is rejected.
- Fail closed: when team-promotion is unreachable, times out or returns an error other than a valid answer,
  `CreateListing` and the featured-ids write return `UNAVAILABLE`; nothing is inserted. A response missing the needed
  limit key is treated as an error (UNAVAILABLE), not as unlimited.
- Never destructive: update, delete and read paths do not consult entitlements; a seller over the cap after a downgrade
  keeps and can edit all existing listings.
- docker-compose: `team-domain` gets `UPSTREAM_PROMOTION_ADDR=team-promotion:<grpc port>` and waits for team-promotion.
- e2e: FREE seller at the cap gets the error on the 11th create; PRO seller is not blocked at 11. Seller-form upgrade
  link is the seller-plan-ui slice; this slice only provides the error code and reason.
- No proto change (consumes existing `GetEntitlements`), no migration (count uses the existing listings table; an index
  on seller_id is added only if the table lacks one, see tasks).

## Capabilities

### New Capabilities
- `listing-plan-cap`: plan-based caps on listing creation and storefront featured ids in team-domain.

### Modified Capabilities
none

## Impact

- team-domain: `internal/config` (new upstream), `internal/listing` (port + check), `internal/store` (count + locked
  create), `internal/rpc/listing.go`, `internal/rpc/storefront.go`, `internal/rpc/errors.go`, `internal/app` wiring.
  docker-compose.yaml (shared hot file with promotion-plan-billing; serialised by initiative-build).
  platform-e2e: scenarios under the listing feature.
- Callers of `CreateListing` (team-gateway, seller UI) see a new error code on the existing RPC; no wire change.
- Add-on dependency: team-promotion must be reachable at startup of listing writes; reads are unaffected.

## NFR

- Latency: the entitlement call adds <= 20 ms p95 to `CreateListing` locally (one in-cluster unary call plus one
  indexed count); upstream call timeout 2 s, after which the request fails with UNAVAILABLE.
- Availability: listing reads, updates and deletes do not depend on team-promotion. Creates and featured-id writes fail
  closed (decision locked 2026-11-13).
- Correctness: under N concurrent creates by one seller with k slots left, exactly k succeed.
- Limits: `max_listings` is an integer, `-1` unlimited, `0` blocks all creates; non-integer values are an error.

## Data classification

No new personal data stored. The seller id is sent to team-promotion to read the plan (existing purpose: seller
billing/entitlements; basis: contract performance). Listing counts are derived, not persisted. Retention: none added.
Raw seller ids are never logged; the rejection log line carries only the id fingerprint (sloglint forbidden-keys).

## Threat model

- Assets: the integrity of plan caps (revenue), seller listing data.
- Abuse cases: (1) a seller bursts parallel creates to overshoot the cap, mitigated by the advisory lock; (2) a seller
  passes another seller's id in the request body, mitigated by the owner being taken from the authenticated principal;
  (3) forcing team-promotion errors to bypass the cap, mitigated by fail-closed; (4) deleting-then-creating to churn is
  allowed by design (cap is on current count).
- Authz rule: only a principal with `listing.write` creates listings and the cap is evaluated for that principal's own
  seller id; entitlements are fetched service-to-service and never exposed through this change.
