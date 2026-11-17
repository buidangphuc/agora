## 1. Code track (team-promotion, team-gateway)

- [x] 1.1 Bump the proto dependency to the monetization-contract version and confirm promotion-ads-serving has landed the `spent`, `impressions`, `clicks`, PAUSED_BUDGET columns/status; `make check` green
- [x] 1.2 Store tests first (Docker, `GO_SDK_REQUIRE_DOCKER=1`): cursor paging, pause/resume conditional UPDATE scoped by `id AND seller_id`, resume-vs-click race; verify red (scenarios: Pagination walks all campaigns without repeats, Resume racing the last click never overspends)
- [x] 1.3 `internal/store/sponsored.go`: `ListBySeller` with cursor (`created_at DESC, id DESC`), `Pause(ctx, id, sellerID)` and `Resume(ctx, id, sellerID)` single conditional UPDATEs returning the row or the prior status (scenarios: Pagination walks all campaigns without repeats, Resume racing the last click never overspends, Boundary remaining budget equal to the bid resumes)
- [x] 1.4 Service tests first in `internal/sponsored`: state machine, clamp, idempotency (scenarios: Oversized page size is clamped, Pausing twice succeeds and changes nothing, Budget-paused campaign can be paused by the seller, Ended campaign cannot be paused, Resuming an active campaign succeeds and changes nothing, Remaining budget below the bid cannot resume, Budget-exhausted campaign cannot be resumed, Ended campaign cannot be resumed); verify red
- [x] 1.5 `internal/sponsored/sponsored.go`: `ListMyAdCampaigns`, `PauseAdCampaign`, `ResumeAdCampaign`; map missing or foreign to ErrNotFound, bad state to FAILED_PRECONDITION (same scenarios as 1.4)
- [x] 1.6 RPC tests first in `internal/rpc`: scope, unauthenticated, NOT_FOUND for foreign and unknown ids, empty id, serving effect (scenarios: Campaigns survive a reload with their counters, Another seller's campaigns are not listed, Seller with no campaigns gets an empty list, Caller without the seller scope is denied, Unauthenticated caller is rejected, Active campaign is paused and stops being served, Another seller's campaign is NOT_FOUND on pause, Unknown campaign id is NOT_FOUND on pause, Empty campaign id is rejected, Caller without the seller scope cannot pause, Paused campaign with budget resumes and is served again, Another seller's campaign is NOT_FOUND on resume, Caller without the seller scope cannot resume); verify red
- [x] 1.7 `internal/rpc/sponsored.go`: handlers using `requireSeller`-style `listing.write` check, principal id as seller, no seller id logged (same scenarios as 1.6); add the NOT_FOUND mapping in `internal/rpc/common.go`
- [x] 1.8 team-gateway tests first then `internal/edge/promotion.go`: forward the three RPCs on `SponsoredForwarder` (scenarios: Anonymous request is rejected at the edge, Upstream NOT_FOUND is passed through, Upstream unavailable is surfaced)
- [x] 1.9 `make check` green in team-promotion and team-gateway

## 2. e2e track (platform-e2e)

- [x] 2.1 Feature `tests/e2e/features/promotion/ad_campaign_management.feature`: "Seller sees campaigns after reload through the gateway" (scenario: Seller sees campaigns after reload through the gateway)
- [x] 2.2 "Seller pauses a campaign through the gateway" and "Seller resumes a paused campaign through the gateway" (scenarios: Seller pauses a campaign through the gateway, Seller resumes a paused campaign through the gateway)
- [x] 2.3 "Resume without budget is refused through the gateway" using a campaign drained by clicks through the real service (scenario: Resume without budget is refused through the gateway)
- [x] 2.4 "Another seller cannot manage a campaign through the gateway" (scenario: Another seller cannot manage a campaign through the gateway)
- [x] 2.5 Register the scenarios in team-promotion FEATURES.yaml (done by the e2e-author)

## 3. Verification

- [x] 3.1 `openspec validate promotion-ads-campaign-mgmt --strict`, spec_lint, team-promotion and team-gateway checks, features check
