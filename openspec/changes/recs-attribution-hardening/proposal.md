## Why

`recsys-online-evaluation` shipped a descriptive report whose review left four follow-ups, all in
`RecommendationPerformance` (team-analytics `internal/query/duckdb.go`):
- purchases are client beacons, so anyone can forge them;
- a click counts even if its listing was never in the impression it names;
- a reused impression id collapses different placements and models into one (`min()` picks one);
- a conversion whose attribution window is cut off by the report end drags the conversion rate down without saying so.

`order-facts-buyer` (previous change) puts the buyer on every order fact, so the server-side truth is now available.

## What Changes

- **team-analytics (`RecommendationPerformance`, `GetRecommendationPerformance`):**
  - **(a)** `purchases` count paid order lines from `order_facts` (buyer = the click's user), not `purchase` beacons.
  - **(b)** a click counts only if its listing was among the impression's listings, at or before the click.
  - **(c)** an impression is identified by (impression id, placement, model version); a reused id yields separate
    impressions; each click is credited to exactly one of them.
  - **(d)** `conversion_rate` is computed over clicks whose attribution window had closed at the report end
    ("mature" clicks); `clicks`, `ctr`, `add_to_carts` and `purchases` still show everything observed.
- **platform-e2e:** scenarios for the new and renamed spec scenarios (`rah_` steps), FEATURES.yaml entries.

Repos touched: team-analytics, platform-e2e (new files). **No proto change**: the response keeps its shape. Surfacing
the number of still-open clicks needs one additive field in platform-core (see design D4); that is deliberately left
to the platform-core owner.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `recsys-online-evaluation`: `purchases` from order facts, click/impression matching, impression identity, and the
  conversion-rate rule.

## Non-goals

- Making `impressions`, `clicks` and `add_to_carts` unforgeable (they stay client beacons; there is no server truth
  for them yet).
- Per-click payout, automatic rollback or any action on the report.
- Counting units or revenue: `purchases` counts order lines, as purchase beacons did.
- Consent, retention or erasure (legal is deferred).
- Back-filling buyers on order facts from before `order-facts-buyer`: those lines are never attributed.
