# [W2-T1] team-gateway — forwarders for all new/changed RPCs

## Role
SE   # gateway routes/forwards only, no business logic

## Objective
Gateway exposes the new RPCs to the edge: a new promotion forwarder, plus updated search (facets), engagement (collections + review extras), notification (alerts), and order (voucher_code) forwarders. 1:1 reflection of gRPC, no aggregation.

## Write-set (EXCLUSIVE)
- team-gateway/internal/edge/promotion.go     (create)
- team-gateway/internal/edge/search.go        (edit — pass through facets + filters)
- team-gateway/internal/edge/engagement.go    (edit — collections + MarkHelpful + shop summary)
- team-gateway/internal/edge/notification.go  (edit — alert subscribe/list)
- team-gateway/internal/edge/order.go         (edit — forward voucher_code)
- team-gateway/internal/edge/*_test.go        (edit/create)

## Read-only dependencies
- team-gateway generated stubs + upstream clients (Wave 0 registered promotion client + edge/server.go path)
- team-gateway/internal/edge/order.go pattern (callRead/callWrite)
- 00-spec.md §Contracts

## Contracts
Forward each RPC verbatim: verify JWT already done upstream; rebuild `x-principal-*` metadata per hop; apply deadlines. No business logic, no cross-service aggregation (that belongs in a BFF, not the gateway).

## Acceptance criteria
- [ ] Promotion voucher + flash-sale RPCs reachable through the gateway edge.
- [ ] Search response carries facets; engagement collections/MarkHelpful/GetShopRatingSummary reachable; notification alerts reachable; order accepts voucher_code.
- [ ] Auth metadata forwarded; unauth calls to write RPCs rejected.
- [ ] gofmt/go vet/go test clean.

## Review (gate — different agent)
Route to **auth-scope-reviewer** (new authed RPCs, principal forwarding) + **contract-boundary-reviewer** (gateway boundary). Require CLEAN.

## Verify
```bash
# in Docker: (cd team-gateway && gofmt -l . && go vet ./... && go test ./...)
```

## Out of scope
- Do NOT edit edge/server.go or upstream/*.go (Wave 0 owns registration). No frontend. No business logic in forwarders.
