## 1. Code track (team-order)

Scenario ids are the scenario titles in `specs/order-completion-lookup/spec.md`.

- [ ] 1.1 Tests first (rpc + store, PG rig): completed / non-completed only / no orders / other buyer / status flip / empty id / user refused / service without scope / no principal / store failure / no buyer id or fingerprint in logs; verify red before 1.2 (scenarios: Buyer with a completed order, Buyer with a completed and a cancelled order, Completed by seller or system counts, Admin refused, Buyer with only non-completed orders, Buyer with no orders, Another buyer's completed order does not count, Status change is visible on the next call, Empty buyer id, Buyer principal refused, Service without the scope refused, Unauthenticated call refused, Store failure is not reported as false, Logs carry no buyer id in any form)
- [ ] 1.2 Store: `HasCompletedOrder(ctx, buyerID) (bool, error)` as one `EXISTS` on `orders` (`buyer_id`, status COMPLETED); add to the fake store (scenarios: Buyer with a completed order, Buyer with no orders)
- [ ] 1.3 Handler `OrderHandler.HasCompletedOrder` in `internal/rpc/order.go`: `RequirePrincipal`, `isServiceWithScope(p, "order.read")`, non-empty `buyer_id`, errors through `h.fail` (scenarios: Empty buyer id, Buyer principal refused, Service without the scope refused, Unauthenticated call refused, Store failure is not reported as false)
- [ ] 1.4 team-gateway edge test beside `TestUnroutedScreenListing`: `HasCompletedOrder` returns 501 and the order upstream sees 0 calls; no production change (scenario: Not routed at the gateway)
- [ ] 1.5 Bump the regenerated proto stubs from the contract slice; `go build`, `go test`, lint, coverage gate via `gate_digest.py` (all scenarios)

## 2. e2e track

- [ ] 2.1 Feature in `platform-e2e/tests/e2e/features/order/`: gateway POST of `HasCompletedOrder` with buyer and admin tokens returns 501 (scenario: Not routed at the gateway)
- [ ] 2.2 Direct gRPC step against team-order with a service token holding `order.read`: false before a buyer's first order completes, true after it completes through the existing checkout and confirm-receipt helpers (scenarios: Buyer with no orders, Status change is visible on the next call, Buyer with a completed order)
- [ ] 2.3 Same step with a user token and a scope-less service token returns PERMISSION_DENIED (scenarios: Buyer principal refused, Service without the scope refused)
- [ ] 2.4 e2e-author adds the `team-order/FEATURES.yaml` entry (rule: only the e2e-author edits it)
