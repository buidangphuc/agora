## Why

initiative: monetization

A seller can create a sponsored campaign today (`SponsoredService.CreateAdCampaign`) but cannot see it again: there is no
list, pause or resume RPC (`promotion.proto` SponsoredService has only `CreateAdCampaign` and `ListSponsoredSlots`), and
the seller campaign table is client state (`team-frontend-seller/src/features/seller/AdCampaignForm.tsx:73`), so it is lost
on reload. User story 5 of the initiative: the seller opens /seller/ads, sees each campaign with listing, bid, budget,
spent, impressions, clicks and status after reload, and can pause and resume it; another seller's campaign is NOT_FOUND.
The proto fields and RPCs are added by monetization-contract; the spent / impressions / clicks counters and the
PAUSED_BUDGET status are written by promotion-ads-serving. This slice implements the three seller-facing RPCs on top of them.

## What Changes

- team-promotion implements `SponsoredService.ListMyAdCampaigns` (the caller's own campaigns, newest first, cursor
  paged), `PauseAdCampaign` and `ResumeAdCampaign` in `internal/rpc/sponsored.go`, `internal/sponsored/sponsored.go` and
  `internal/store/sponsored.go` (`ListBySeller` gains a cursor; two conditional status transitions are added).
- Authorisation: seller scope `listing.write` (same rule as `CreateAdCampaign`); a campaign owned by another seller, or a
  missing one, is NOT_FOUND (existence is not revealed). No admin override in v1.
- State machine (status changes only by these conditional UPDATEs, never read-then-write):
  Pause: ACTIVE -> PAUSED, PAUSED -> PAUSED (idempotent), PAUSED_BUDGET -> PAUSED, ENDED -> FAILED_PRECONDITION.
  Resume: PAUSED -> ACTIVE only when `budget - spent >= bid`, else FAILED_PRECONDITION; ACTIVE -> ACTIVE (idempotent);
  PAUSED_BUDGET and ENDED -> FAILED_PRECONDITION (a budget-exhausted campaign cannot be resumed; the seller creates a new one).
- team-gateway `internal/edge/promotion.go` forwards the three RPCs on `SponsoredForwarder` with the caller's token (seller
  session required).
- e2e: one feature file covering list after reload, pause, resume, cross-seller NOT_FOUND, resume with no budget.
- No proto change (monetization-contract), no new table or column (promotion-ads-serving owns `spent`, `impressions`,
  `clicks`, `category_id`, PAUSED_BUDGET), no wallet movement, no UI (seller-ads-ui).

## Capabilities

### New Capabilities
- `promotion-ads-campaign-mgmt`: the seller's own campaign list, pause and resume, with ownership isolation.

### Modified Capabilities
none (the existing `promotion-access-control` requirements cover other RPCs; this slice adds new ones and does not restate them).

## Impact

- team-promotion: `internal/rpc/sponsored.go`, `internal/sponsored/sponsored.go`, `internal/store/sponsored.go` and tests.
- team-gateway: `internal/edge/promotion.go` (hot file shared with promotion-plan-billing and promotion-ads-serving; serialised by initiative-build).
- platform-e2e: one feature file plus steps.
- Depends on promotion-ads-serving for the columns and PAUSED_BUDGET; seller-ads-ui consumes these RPCs.

## NFR

- Latency: `ListMyAdCampaigns` p95 <= 50 ms locally for a page of 50 (single indexed query on `(seller_id, created_at DESC)`);
  pause / resume p95 <= 30 ms (one conditional UPDATE).
- Availability: no new upstream; the three RPCs depend only on promotion_db. They never call team-payment or team-domain
  (ownership comes from `seller_id` on the campaign row, not from a GetListing call).
- Limits: page_size default 50, clamped to 100; campaigns per seller are not capped in v1.
- Consistency: pause / resume are single conditional UPDATEs, so a concurrent click that flips the campaign to
  PAUSED_BUDGET is never overwritten by a resume (no overspend, no lost transition).
- Money: integer VND, int64; remaining budget is `budget - spent`, no rounding.

## Data classification

| field | class | purpose | consent / basis | retention |
|---|---|---|---|---|
| `ad_campaigns.seller_id` | personal (pseudonymous id) | ownership check and "my campaigns" filter | contract performance (seller's own ads account) | life of the campaign row; deleted with the seller account per the existing erasure process |
| bid, budget, spent, impressions, clicks, status, listing_id | business, not personal | campaign management and display to its owner | contract performance | same as the campaign row |

No buyer data is read or returned by these RPCs. The seller id is never written to logs (sloglint forbidden keys); logs
carry the campaign id only.

## Threat model

- Assets: a seller's campaign list and spend figures (competitive information); the ability to start or stop serving paid slots.
- Abuse cases: (1) seller A lists, pauses or resumes seller B's campaign by guessing the id (IDOR); (2) a buyer or
  anonymous caller reaches the seller RPCs; (3) a resume racing a budget-exhausting click re-activates a drained campaign
  and overspends; (4) enumeration of campaign ids by distinguishing PERMISSION_DENIED from NOT_FOUND; (5) unbounded page size.
- Authz rule: the caller must be authenticated and hold `listing.write`; the target campaign must have `seller_id` equal to
  the principal id, else NOT_FOUND. The list filters by the principal id and never accepts a seller id from the request.
  A missing scope is PERMISSION_DENIED, no token is UNAUTHENTICATED. Transitions are conditional UPDATEs scoped by
  `id AND seller_id`.
