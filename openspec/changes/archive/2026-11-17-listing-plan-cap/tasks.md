## 1. Code track (team-domain)

- [x] 1.1 Tests first in `internal/config`: required `UPSTREAM_PROMOTION_ADDR`, loaded and declared (scenarios: Missing address fails startup, Configured address is declared in settings); verify red
- [x] 1.2 Tests first in `internal/listing` with a fake entitlement port: cap reject, below cap, unlimited, zero cap, over-cap keeps listings, delete re-enables (scenarios: FREE seller at the cap is rejected, Create just under the cap succeeds, Unlimited plan is never blocked, Zero cap blocks every create, Over-cap seller keeps listings and can edit them, Deleting below the cap re-enables create); verify red
- [x] 1.3 Tests first for fail-closed: unavailable, timeout, malformed limits (scenarios: team-promotion down blocks create, Upstream timeout blocks create, Malformed limits are not unlimited)
- [x] 1.4 `internal/config`: add `UpstreamPromotionAddr` to `Upstream`, validate non-blank, declare in settings (scenarios: both config scenarios)
- [x] 1.5 `internal/listing`: `EntitlementsPort` (small interface) plus gRPC adapter to `SubscriptionService.GetEntitlements` using the service principal, 2 s timeout; parse `max_listings` / `featured_listings` (`-1` unlimited, error on missing or non-integer); wire in `internal/app`
- [x] 1.6 `internal/store`: per-seller count of all listings and a create path that takes a per-seller advisory lock, counts and inserts in one transaction; add a seller_id index only if missing (scenarios: Every status counts toward the cap, Concurrent creates cannot overshoot the cap)
- [x] 1.7 Store integration tests (Docker, `GO_SDK_REQUIRE_DOCKER=1`): all-statuses count, 6 concurrent creates with 2 slots left (scenarios: Every status counts toward the cap, Concurrent creates cannot overshoot the cap)
- [x] 1.8 `internal/rpc/listing.go` and `errors.go`: map the cap error to RESOURCE_EXHAUSTED with `errdetails.ErrorInfo{Reason: "PLAN_LISTING_LIMIT"}` and entitlement failures to UNAVAILABLE; scope check stays first; owner from principal (scenarios: Over-cap status does not let another seller touch the listings, Another seller's storefront cannot be written through the cap path, Cap is evaluated for the caller's own seller id, Caller without listing.write is refused before any entitlement call, Update, delete and read work while team-promotion is down)
- [x] 1.9 Storefront: tests first then enforce `featured_listings` in `UpsertStorefront`, reason `PLAN_FEATURED_LIMIT`, entitlement lookup only when featured ids are present, remove any fixed limit of 6 (scenarios: all Storefront featured scenarios)
- [x] 1.10 docker-compose.yaml: `UPSTREAM_PROMOTION_ADDR` for team-domain and dependency on team-promotion (shared hot file, rebase on promotion-plan-billing)
- [x] 1.11 Log hygiene: rejection log carries id fingerprint only; `make check` green

## 2. e2e track (platform-e2e)

- [x] 2.1 Feature `listing/listing_plan_cap.feature` "FREE seller is blocked at the 11th listing end to end" with seeded seller at the cap via the real service (scenario: FREE seller is blocked at the 11th listing end to end)
- [x] 2.2 Scenario "PRO seller is not blocked at the 11th listing end to end" (scenario: PRO seller is not blocked at the 11th listing end to end)
- [x] 2.3 Scenario "Over-cap seller keeps their listings end to end" (scenario: Over-cap seller keeps their listings end to end)
- [x] 2.3b Scenario "Another seller's listings are not exposed to the capped seller" (scenario: Another seller's listings are not exposed to the capped seller)
- [x] 2.4 Register the scenarios in team-domain FEATURES.yaml under `listing.create` (done by the e2e-author)

## 3. Verification

- [x] 3.1 `openspec validate listing-plan-cap --strict`, spec_lint, team-domain checks, features check
