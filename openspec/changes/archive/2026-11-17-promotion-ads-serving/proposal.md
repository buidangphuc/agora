## Why

initiative: monetization

Sponsored listings exist only as a seller-side create call. `ListSponsoredSlots` ignores its context and returns the top 50
active campaigns by bid (`team-promotion/internal/sponsored/sponsored.go:128`, `internal/store/sponsored.go:124-128`),
the budget never moves, no page can tell a click from an impression, and a bid of 0 is accepted. This slice makes
team-promotion serve placement-aware sponsored slots with single-use impression tokens and charge each click against the
campaign budget exactly once, so the buyer frontend slice (`buyer-sponsored-slots`) can show "Tài trợ" slots and the platform
earns per click. Decisions are locked in `plans/monetization/00-spec.md`: CPC, first-price, minimum bid 500 VND, budget is a
bookkeeping counter (no wallet debit, no AD_SPEND entry), 2 slots on search page 1 and 2 on home, category + bid ranking.

## What Changes

- `CreateAdCampaign` (existing RPC): bid below `MIN_AD_BID` (500) is `INVALID_ARGUMENT`; budget below bid is
  `INVALID_ARGUMENT`; the listing's `category_id` is read from the `GetListing` call the ownership check already makes and
  stored on the campaign. The admin path keeps its no-team-domain-lookup rule, so its campaigns store an empty category.
- `ListSponsoredSlots` (existing RPC, new request fields `placement`, `category_id`, `limit`; new response `slots`): serves
  ACTIVE campaigns with remaining budget >= bid, for `placement` `search` (category must match) or `home` (any category),
  ranked bid descending then oldest campaign first, at most one slot per listing, `limit` 1..4 (default 2). Each served slot
  carries a random `impression_token`, persisted in the new table `ad_impressions` (no buyer id, IP or session), valid 30
  minutes, and the campaign's `impressions` counter is incremented. `listing_ids` stays populated for old callers.
- `RecordAdClick(impression_token)` (new RPC, callable anonymously): in one transaction it consumes the token and charges the
  campaign's bid once (`spent += bid`, `clicks += 1`) with a compare-and-set on remaining budget; when remaining budget
  falls below the bid the campaign becomes `PAUSED_BUDGET`. Replayed, expired, self-clicked and paused-campaign clicks charge
  nothing and still return the listing id so navigation works.
- New campaign columns `spent`, `impressions`, `clicks`, `category_id`; new status `AD_CAMPAIGN_STATUS_PAUSED_BUDGET = 4`.
  Migration backfills existing rows and ends legacy campaigns whose bid is below the minimum.
- The existing sweeper loop (`internal/consumer/sweeper.go`) purges `ad_impressions` rows older than 30 days.
- team-gateway forwards `SponsoredService.RecordAdClick` anonymously (no scope), like `ListSponsoredSlots`. It holds no ad logic.
- e2e: search and home slots, click charge, replay, budget exhaustion, self-click through the real stack.
- Not here: `ListMyAdCampaigns` / `Pause` / `Resume` (promotion-ads-campaign-mgmt), buyer UI and organic de-duplication
  (buyer-sponsored-slots), proto edits (monetization-contract), wallet debit (non-goal in v1).

## Capabilities

### New Capabilities
- `promotion-ads-serving`: placement-aware sponsored slot serving with impression tokens and per-click budget charging.

### Modified Capabilities
none. The existing `promotion-access-control` rule "Plan subscription and sponsored ads need seller rights" is extended
(minimum bid) but its text and scenarios stay true; the minimum-bid rule lives in this capability.

## Impact

- team-promotion: `internal/sponsored`, `internal/store` (repo + migration, next free number after the other promotion
  slices, serialized by initiative-build), `internal/rpc/sponsored.go`, `internal/rpc/common.go` (status mapping),
  `internal/consumer/sweeper.go`, config `MIN_AD_BID`. team-gateway: `internal/edge/promotion.go` (one forwarder method).
  platform-e2e: new feature file plus steps.
- Callers of `ListSponsoredSlots` that send no `placement` keep the legacy answer (see design).

## NFR

- Latency: `ListSponsoredSlots` p95 <= 30 ms locally for `limit` <= 4 (one indexed read plus one batched token insert).
  `RecordAdClick` p95 <= 50 ms locally.
- Correctness under concurrency: the click charge and budget update are one compare-and-set; N concurrent clicks on
  distinct tokens of one campaign with budget for K clicks charge exactly K, and `spent` never exceeds `budget`. One token
  charges at most once.
- Availability: if `ListSponsoredSlots` errors the buyer page renders organic results (owned by buyer-sponsored-slots); a
  failed click charge never blocks navigation because the response is not on the navigation path.
- Limits: `limit` max 4; token TTL 30 minutes; `ad_impressions` retention 30 days; `RecordAdClick` is rate limited per
  client by the existing gateway edge limiter.
- Money: integer VND, int64, no fractions; no wallet movement.

## Data classification

| Field | Where | Purpose | Basis | Retention |
|---|---|---|---|---|
| seller id | `ad_campaigns.seller_id` (existing) | campaign ownership, self-click check | contract performance (seller bought the ad) | life of the campaign |
| impression token, campaign id, listing id, timestamps | `ad_impressions` | single-use click billing | legitimate interest: prevent double charging; no personal data | purged after 30 days |
| buyer id, IP, session, user agent | not stored | n/a | n/a | n/a |

The self-click check compares the forwarded principal id with the campaign's seller id in memory only; the buyer id is
never persisted or logged. Logs carry no raw seller, buyer or token values (sloglint forbidden-keys).

## Threat model

- Assets: campaign budgets (a counter that will back real money later), advertiser trust in click counts.
- Abuse cases: (1) replaying a click token to drain a competitor's budget: the token is consumed in the same transaction as
  the charge, so only the first click charges; (2) minting tokens: tokens are random 128-bit values only issued by
  `ListSponsoredSlots` and looked up server-side, a guessed token is `NOT_FOUND` and charges nothing; (3) a seller clicking
  their own ad to inflate stats: principal equals campaign seller, nothing charged and the token stays usable; (4) a
  seller clicking a rival's ad through many anonymous sessions: tokens are per impression and each charge is capped by
  the rival's own budget, rate limiting at the edge bounds the rate; residual risk accepted in v1 (no fraud model);
  (5) concurrent clicks overspending: single compare-and-set; (6) enumerating campaigns through `ListSponsoredSlots`:
  it returns only listing id, campaign id and token, never seller, bid or budget.
- Authz rule: `ListSponsoredSlots` and `RecordAdClick` are public (anonymous allowed, no scope); `CreateAdCampaign` keeps
  `listing.write` plus ownership. Neither public RPC returns or accepts a seller id; the principal, when present, is
  only compared with the campaign's seller.
