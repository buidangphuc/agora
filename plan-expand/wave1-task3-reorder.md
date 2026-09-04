# [W1] F3 Reorder / Mua lại (team-order)

## Role
SE

## Objective
Implement the RPC(s) below in **team-order** end-to-end (handler → service → repository + migration) with unit tests, green on host.

## Write-set (EXCLUSIVE — nothing outside team-order/)
- team-order/** (internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (Wave 0)
- Reference in this repo: CartService.AddToCart + OrderService.GetOrder

## Contracts you implement
`Reorder(order_id)` -> re-adds the items of a past order to the caller cart, returns the updated Cart.

## Reference implementation
Mirror **CartService.AddToCart + OrderService.GetOrder**: same handler/service/repository layering + test layout. No new table; reads the order items and calls the existing cart add path. Owner-scoped (only reorder your own order).

## Acceptance criteria
- [ ] Real DB logic (InMemory + Postgres split so tests run without a live DB); auth-scoped where user-owned
- [ ] Anonymous/empty-input handled without panic
- [ ] >=3 tests (happy + 2 edges incl. cross-user isolation where relevant)
- [ ] Verify green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-order && buf generate && go test ./...

## Out of scope
- Do NOT touch team-gateway, team-frontend, other repos (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0) or hand-edit generated code. Money stays mock.
