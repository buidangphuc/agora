## 1. Code track (team-promotion, team-gateway)

- [x] 1.1 Bump the proto dependency to the monetization-contract version in team-promotion and team-gateway; regenerate; build green (all scenarios)
- [x] 1.2 Tests first for creation rules in `internal/sponsored`: min bid, budget below bid, foreign listing, category stored; verify red (scenarios: Bid at the minimum is accepted, Bid below the minimum is rejected, Budget below the bid is rejected, Another seller cannot advertise the listing)
- [x] 1.3 Migration (next free number) adding campaign columns, `ad_impressions`, indexes, legacy backfill, down file; Docker tests (`GO_SDK_REQUIRE_DOCKER=1`) (scenarios: Legacy campaigns that could never serve are ended, The migration reverses cleanly, The stored row holds no buyer data)
- [x] 1.4 Tests first for serving: ranking, tie, category, home, dedupe, limit bounds, unknown placement, legacy call, non-servable statuses (scenarios: Search serves the best two campaigns of the category, Equal bids favour the oldest campaign, Other categories are excluded from search, Search without a category returns no slots, Home serves across categories, A listing with two campaigns appears once, Paused, budget-exhausted and ended campaigns are not served, Limit boundary, Unknown placement is rejected, Legacy caller without placement keeps the old answer); verify red
- [x] 1.5 Implement `MIN_AD_BID` config, `AD_CAMPAIGN_STATUS_PAUSED_BUDGET` mapping, repository `ListServable` and batched `IssueImpressions` (one transaction), `sponsored.Service.ListSponsoredSlots` and `CreateAdCampaign` changes (scenarios of 1.2 and 1.4, Serving records one impression per slot, Empty result writes nothing, A failed token insert fails the call without counting)
- [x] 1.6 Tests first for clicks: charge once, replay, expired, unknown, empty token, own seller, other principal, exhaustion pause, paused campaign, rollback, concurrency (scenarios: First click charges the bid, Replaying the token charges nothing, Expired token charges nothing, Unknown token is not found, Empty token is invalid, A seller clicking their own ad is not charged, Another seller or a buyer clicking is charged, The last affordable click pauses the campaign, A click on a campaign that can no longer pay charges nothing, Concurrent clicks never overspend, Concurrent clicks on one token charge once, A failed charge rolls the token back); verify red
- [x] 1.7 Implement `RecordAdClick` in `internal/sponsored`, the repository compare-and-set transaction, and `internal/rpc/sponsored.go` handlers with an optional principal and no scope (same scenarios as 1.6, Slots expose no seller or money data)
- [x] 1.8 Sweeper: purge `ad_impressions` older than 30 days in batches, error isolated from the voucher sweep (scenarios: Rows older than 30 days are deleted, Purge failure does not stop the voucher sweep)
- [x] 1.9 team-gateway `internal/edge/promotion.go`: `SponsoredForwarder.RecordAdClick`, tests first (scenarios: Anonymous caller reaches RecordAdClick, A logged-in buyer's principal is forwarded, Malformed bearer token is rejected)
- [x] 1.10 `make check` green in team-promotion and team-gateway; no raw seller, buyer or token values in logs

## 2. e2e track (platform-e2e)

- [x] 2.1 Feature `ads/sponsored_serving.feature`: seller with an ACTIVE campaign, search by its category returns a tokened slot, home returns it too (scenario: Served slots are visible end to end)
- [x] 2.2 Feature `ads/sponsored_click.feature`: double click charges once, then budget exhaustion removes the listing from serving; campaign state read via the real service, mock data through the service's own API, no fake containers (scenarios: Click and budget burn end to end, Exhausted campaign stops being served)
- [x] 2.3 Register both under team-promotion FEATURES.yaml (done by the e2e-author)

## 3. Verification

- [x] 3.1 `openspec validate promotion-ads-serving --strict`, spec_lint, team-promotion and team-gateway checks, features check
