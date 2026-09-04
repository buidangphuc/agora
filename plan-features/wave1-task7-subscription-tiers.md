# [W1] F7 Seller subscription tiers (team-promotion)

## Role
SE

## Objective
Implement the RPC(s) below in **team-promotion**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-promotion/)
- team-promotion/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: the voucher service (list + redeem) in internal/{handler,service,repository}

## Contracts you implement
`ListPlans`, `Subscribe` (mock — no charge), `GetEntitlements` (feature flags/limits per tier: FREE/PRO/PREMIUM).

## Reference implementation
Mirror **the voucher service (list + redeem) in internal/{handler,service,repository}** in team-promotion: same handler/service/repository layering, same test layout, new domain. Migration: subscription_plans (seed FREE/PRO/PREMIUM), seller_subscriptions(seller_id, plan_id, status).

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-promotion && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
