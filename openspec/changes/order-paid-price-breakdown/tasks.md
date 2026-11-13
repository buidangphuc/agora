## 1. Code track (team-order)

- [ ] 1.1 Tests first in `internal/events`: shop-only, shop+platform split, zero-discount presence, legacy fields unchanged (scenarios: Shop voucher only, Shop and platform voucher split, No discount sets explicit zeros, Existing fields and event id are unchanged); verify red
- [ ] 1.2 Tests first for the guard: negative shop share, incoherent total, counter increments (scenarios: Platform share larger than the discount is skipped, Amounts that do not sum to the total are skipped); verify red
- [ ] 1.3 `internal/events/publisher.go`: fill fields 9-12 in `BuildOrderPaidEnvelope`, add the coherence guard and `order_paid_breakdown_skipped_total` (scenarios: all of requirements 1 and 2)
- [ ] 1.4 Store integration tests (Docker, `GO_SDK_REQUIRE_DOCKER=1`): breakdown in the PAID outbox row, legacy pending row relayed unchanged, replay keeps one row (scenarios: Breakdown is committed in the PAID transaction, A failing enqueue rolls the PAID transition back, Outbox rows enqueued before deploy are not rewritten, Re-emitted PAID fact collapses to one row)
- [ ] 1.5 Bump the proto dependency to the monetization-contract version; `make check` green

## 2. e2e track (platform-e2e)

- [ ] 2.1 Feature scenario "Paid order event carries its price breakdown" in `tests/e2e/features/order/order_events_published.feature` with steps (scenario: Paid order's event matches GetOrder)
- [ ] 2.2 Feature scenario "Paid order without voucher reports zero discounts" (scenario: Event for an order without a voucher reports zero discounts)
- [ ] 2.3 Register both in team-order FEATURES.yaml under `order.events-published-on-paid` (done by the e2e-author)

## 3. Verification

- [ ] 3.1 `openspec validate order-paid-price-breakdown --strict`, spec_lint, team-order checks, features check
