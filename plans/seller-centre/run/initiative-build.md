# initiative-build — seller-centre
root: /Users/phuc.buidang/Documents/agora-seller-centre (feat/seller-centre)
integration: .claude/worktrees/seller-centre (asdlc/seller-centre)
wip_limit: 3
LOCK digests verified 2026-11-13: 14/14 match.
| wave | slices | base | status |
|---|---|---|---|
| 0 | seller-centre-contract | asdlc/seller-centre | pending |
| 1 | domain-my-listings-filter, order-seller-list-paging, promotion-seller-self-service | asdlc/seller-centre after wave 0 | pending |
| 2 | domain-storefront-customisation, seller-server-paging-ui | asdlc/seller-centre after wave 1 | pending |
| 3 | domain-bundle-integrity, seller-shop-customisation-ui, shop-page-featured-theme | asdlc/seller-centre after wave 2 | pending |
| 4 | order-bundle-checkout, seller-marketing-hub | asdlc/seller-centre after wave 3 | pending |
| 5 | seller-getting-started, shop-bundle-purchase-ui | asdlc/seller-centre after wave 4 | pending |
| 6 | seller-bulk-ship | asdlc/seller-centre after wave 5 | pending |
Per-slice state: plans/seller-centre/run/<slice>.md

## Wave rationale
- DAG levels from slices.yaml; the contract slice runs alone.
- Serialised by shared files (00-spec hot files plus each owner's FEATURES.yaml): the three team-domain slices (store port, `rpc/listing.go`, `rpc/errors.go`) run in waves 1, 2, 3; the five team-frontend-seller slices run one per wave (2-6); team-frontend slices in 3, 4 (vouchers page), 5; team-gateway edge (promotion.go wave 1, cart.go wave 4); team-order store (waves 1, 4).
- seller-server-paging-ui goes first among seller UI slices: it adds the shared `ListingPicker` / `searchMyListingsAction` that seller-shop-customisation-ui and seller-marketing-hub reuse (LOCK cross-slice decision 6).

## Carry-forward
- Migration number clash: order-seller-list-paging (wave 1) and order-bundle-checkout (wave 4) both name `0025_*` in team-order. Wave 1 takes 0025; order-bundle-checkout must use the next free number (packet write-set is `*_bundle_lines.*`). Its tasks.md says `0025_bundle_lines`: traceability-only spec edit → NEEDS_HUMAN re-lock at step 5.
- LOCK.md "Open issues" decisions (cross-slice 1-7 and per-slice) are not resolved in the specs; feature-run takes the spec defaults and routes a blocking one to spec-clarify (human).
- Worktrees have no platform-e2e/.venv or node_modules: symlink /Users/phuc.buidang/Documents/agora/platform-e2e/.venv (never commit it); generate stubs in the worktree (team-gateway `buf generate --template buf.gen.local.yaml`, FE `npm ci && npm run proto`).
- Go DB tests: DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1.
