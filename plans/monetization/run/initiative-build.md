# initiative-build — monetization
root: /Users/phuc.buidang/Documents/agora-monetization (feat/monetization)
integration: .claude/worktrees/monetization (asdlc/monetization)
wip_limit: 3
lock: plans/monetization/LOCK.md approved 2026-11-13; all 12 slice digests re-checked equal at build start
| wave | slices | base | status |
|---|---|---|---|
| 0 | monetization-contract | asdlc/monetization | pending |
| 1 | order-paid-price-breakdown, payment-seller-fee-charge, listing-plan-cap, promotion-ads-serving | asdlc/monetization after wave 0 | pending |
| 2 | payment-settlement-commission, promotion-ads-campaign-mgmt, buyer-sponsored-slots | asdlc/monetization after wave 1 | pending |
| 3 | promotion-plan-billing, seller-fee-breakdown-ui | asdlc/monetization after wave 2 | pending |
| 4 | seller-ads-ui | asdlc/monetization after wave 3 | pending |
| 5 | seller-plan-ui | asdlc/monetization after wave 4 | pending |
Per-slice state: plans/monetization/run/<slice>.md

## Wave rationale
- Wave 0: contract slice alone (proto + 35 vendored copies + fe-platform api-client stubs).
- Waves 1-2 follow the depends_on DAG levels; payment-settlement-commission waits for payment-seller-fee-charge (ledger.go + ledger CHECK migration).
- team-promotion chain is serialised: promotion-ads-serving (1) -> promotion-ads-campaign-mgmt (2) -> promotion-plan-billing (3) on migrations, sponsored.go, store/sponsored.go and team-gateway edge/promotion.go.
- docker-compose.services.yaml: listing-plan-cap (1) before promotion-plan-billing (3).
- team-frontend-seller: seller-fee-breakdown-ui (3), seller-ads-ui (4), seller-plan-ui (5) each own team-frontend-seller/FEATURES.yaml; seller-ads-ui and seller-plan-ui also share lib/gateway/promotion.ts and features/seller/actions.ts, so they cannot share a wave (LOCK proposed 4 waves; the FEATURES.yaml overlap adds wave 5).
- validate_plan.py plans/monetization: 12 packets, 6 waves, write-sets disjoint per wave.

## Carry-forward
- Worktrees have no platform-e2e/.venv: symlink /Users/phuc.buidang/Documents/agora/platform-e2e/.venv into the worktree's platform-e2e/ (never commit it). Generated stubs are gitignored: team-gateway `buf generate --template buf.gen.local.yaml`; FE apps `npm ci && npm run proto`.
- Go DB tests: DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1.
- team-gateway/README.md lists services, not RPCs: no gateway README edit expected; it is in no packet.
- promotion-plan-billing's placeholder migration 0009 is renumbered after promotion-ads-serving's migration; payment-settlement-commission's migration follows payment-seller-fee-charge's and keeps its SUBSCRIPTION_FEE arm.
- LOCK.md NEEDS_HUMAN decisions 1-14 run on their bracketed defaults; DPO sign-off on 12-month lapsed subscription retention is still open.
