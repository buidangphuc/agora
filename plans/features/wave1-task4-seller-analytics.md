# [W1] F4 Seller analytics (team-analytics)

## Role
SE

## Objective
Implement the RPC(s) below in **team-analytics**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-analytics/)
- team-analytics/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: the existing grpcserver + a repository that queries the warehouse; mirror another Go service grpcserver registration

## Contracts you implement
Add a query gRPC service to team-analytics (currently health + Kafka consumer only): `GetSellerFunnel` (impressions→views→adds→orders), `GetRevenueBreakdown` (by day / top SKUs). Reads the warehouse tables the consumer already populates.

## Reference implementation
Mirror **the existing grpcserver + a repository that queries the warehouse; mirror another Go service grpcserver registration** in team-analytics: same handler/service/repository layering, same test layout, new domain. No new table (reads warehouse); add read queries + a grpc query servicer.

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-analytics && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
