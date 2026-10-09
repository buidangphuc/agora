## Context

See proposal.md. `RecommendationPerformance` is one DuckDB query over `tracking_events_resolved` (D1 of
`recsys-online-evaluation`): `imp` (grouped by impression_id with `min()` of placement and model), `clk`, `conv`.
Impression events carry one listing each, and share the impression id of the list they belong to. Click beacons carry
the impression id and may carry placement and model (the storefront does not today). `order_facts` has
`buyer_id` (previous change `order-facts-buyer`), `listing_id`, `occurred_at` (paid time, server clock), `status`.

## Goals / Non-Goals

**Goals:** server-truth purchases; no credit for clicks the impression cannot explain; no collapse of reused ids; a
conversion rate that is not biased by open windows.
**Non-Goals:** see proposal.

## Decisions

### D1 (a). Purchases come from `order_facts`
A purchase is one `order_facts` line with `status = 'PAID'` and a non-empty `buyer_id` equal to the click's `user_key`
(logged-in users, and anonymous visitors stitched to a user), the same `listing_id`, and `occurred_at` between the click
and the click plus the attribution window, and not after the report end. Each line is credited once, to the earliest
attributable click (as before). `purchase` beacons are ignored. `add_to_carts` stay beacon-based: nothing server-side
in the warehouse records carts. Lines with a NULL buyer (before `order-facts-buyer`) are never attributed. One order
line is one purchase (not quantity), matching one beacon per purchase before.

### D2 (b)+(c). Impression identity and click matching
- Impression identity is the triple (impression id, placement id, model version); a NULL model version is `''`.
  `impressions` = distinct triples, `item_impressions` = impression events of those triples. The same id under another
  placement or model is another impression in another row. (Today `min()` silently drops one of them.)
- A click is matched to impression events with the same impression id whose listing is the clicked listing and whose
  time is at or before the click. If the click carries a placement or model version, they must equal the
  impression's; an empty one matches any (it does today). Among the candidates the click is credited to the latest
  impression event; ties break by placement id, then model version, ascending, so the result is deterministic.
  A click with no candidate is not counted anywhere.
- Impressions are still taken from the report window only, so a click at the start of the window whose impression is
  just before it is not counted (unchanged).
Alternative rejected: credit the click to every matching triple. It inflates clicks and double-credits purchases.

### D3 (d). Mature clicks and the conversion rate
A click is mature when `click time + attribution window <= report end`. `conversion_rate` =
(purchases credited to mature clicks) / (mature clicks), 0 with no mature clicks. All other numbers include open
clicks, so a purchase right after a click is still counted in `purchases`.
Alternatives rejected:
- Dropping conversions of open clicks entirely: with the defaults (24 h window, 24 h attribution) it would zero every
  purchase, and break the "purchase right after the click is attributed" scenario.
- Leaving it as is: the newest clicks always lower the rate, and the admin cannot tell.
Consequence: with `window_hours <= attribution window` there is no mature click and the rate is 0; the report needs a
wider `window_hours` to show a rate. This is stated in the spec.

### D4. Surfacing the open count needs the contract
The honest form of "reported as such" is a per-row `open_window_clicks` (and a proto comment fixing
`conversion_rate`'s definition). That is an additive field on `RecommendationPerformanceRow` in platform-core,
vendored into team-analytics, team-gateway and team-frontend. This change does not edit the contract
(AGENTS.md rule 4). The repository row already carries `MatureClicks`, so once the field exists the service sets it
from `Clicks - MatureClicks` in one line. Until then the proto comment on `conversion_rate` ("purchases / clicks") is
stale; the spec states the rule.

## Risks / Trade-offs

- Ingest lag: a paid order is in `order_facts` a few seconds after payment (Kafka), so a purchase can appear after the
  click's report. Mitigation: none needed for a descriptive report; e2e polls.
- Impressions and clicks remain forgeable → documented non-goal; purchases (the money number) no longer are.
- Clients whose clock is behind the impression's (click earlier than the impression) are not counted; both stamps come
  from one browser, so skew within a session is zero in practice.
- Performance: one more join on `order_facts` by (buyer_id, listing_id) over a window; fine at local scale.

## Migration Plan

Deploy team-analytics only (no schema change; `order-facts-buyer` must be deployed first, otherwise no purchase is
attributed). Rollback restores beacon purchases. The e2e scenario that posted a purchase beacon
("A purchase after a recommended click is attributed") keeps its name but now pays a real order; the roe copy must be removed.
