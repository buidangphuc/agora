## 1. Code track (team-promotion, team-gateway, docker-compose)

- [ ] 1.1 Tests first, store: migration price correction, catalog equals `DefaultPlans()`, grandfathering, concurrent subscribe, verify red (scenarios: Catalog shows corrected prices, Go catalog and migrated rows agree, Non-positive term is rejected by the schema, Existing paid subscription is grandfathered for one term, Concurrent subscribes charge once)
- [ ] 1.2 Migration `0009_plan_billing` (renumber at build) plus `DefaultPlans()` prices/term and repository methods (lock-for-update get, period update, due-for-renewal select) in `internal/store/subscription.go` and the storefake (same scenarios)
- [ ] 1.3 Tests first, service with a fake payment client: upgrade, no proration, insufficient balance, unavailable, retry replays key, deferred downgrade, cancel, cancel on FREE, resume (scenarios: FREE seller subscribes to PRO, Insufficient withdrawable balance is rejected, Payment service unavailable fails without state change, Retry after a lost response is not charged twice, Upgrade charges the full price without proration, Downgrade is deferred, Cancel keeps the plan until the end, Cancel without a paid plan is rejected, Cancel without seller scope is denied, Re-subscribing to the current plan resumes renewal); verify red
- [ ] 1.4 `internal/upstream/payment.go` (service principal, `ChargeSellerFee`), `internal/subscription` Subscribe/Cancel/GetMySubscription per design D1-D4 (same scenarios)
- [ ] 1.5 Tests first, sweeper: renew once, cancelled lapse, pending downgrade, FAILED_PRECONDITION lapse, transient retry inside and past window, concurrent sweepers (scenarios of the renewal requirement); then extend `internal/consumer/sweeper.go` and the `subscription_renewals_overdue` metric
- [ ] 1.6 RPC layer tests then `internal/rpc/subscription.go`: `GetMySubscription`, `CancelSubscription`, scope and input checks, own-id only (scenarios: Missing or unknown plan id is invalid, Subscribe without seller scope is denied, Paid seller reads own subscription, FREE seller gets an empty response, Another seller's subscription is not reachable)
- [ ] 1.7 Config: `UPSTREAM_PAYMENT_ADDR`, `UPSTREAM_PAYMENT_TIMEOUT`, `SUBSCRIPTION_RENEWAL_RETRY_WINDOW`, app wiring, README env table (scenarios: Blank payment address is rejected at startup, Non-positive timeout is invalid)
- [ ] 1.8 team-gateway `internal/edge/promotion.go`: forwarders for the two RPCs, auth, no `ChargeSellerFee` route (scenarios: Unauthenticated call is denied at the edge, Gateway forwards the new RPCs and never ChargeSellerFee)
- [ ] 1.9 `docker-compose.services.yaml`: `UPSTREAM_PAYMENT_ADDR` on team-promotion; bump the proto dependency to the `monetization-contract` version; `make check` green with `GO_SDK_REQUIRE_DOCKER=1`

## 2. e2e track (platform-e2e)

- [ ] 2.1 `features/promotion/seller_plan_billing.feature`: upgrade debits the wallet (scenario: Seller upgrades to PRO and the wallet is debited)
- [ ] 2.2 Same file: insufficient funds (scenario: Seller without funds cannot upgrade)
- [ ] 2.3 Same file: cancel then lapse, using the stack's sweep interval or a real-service guarded mode for a short term, no route fulfils (scenario: Cancelled plan lapses at period end)
- [ ] 2.3b Same file: isolation between sellers (scenario: Another seller cannot cancel my plan)
- [ ] 2.4 Register in team-promotion FEATURES.yaml (done by the e2e-author)

## 3. Verification

- [ ] 3.1 `openspec validate promotion-plan-billing --strict`, spec_lint, team-promotion and team-gateway checks, features check
