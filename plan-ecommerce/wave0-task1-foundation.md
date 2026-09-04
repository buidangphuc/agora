# [W0-T1] Foundation — proto contracts, team-promotion skeleton, shared registrations

## Role
SA   # structural decisions; owns all shared/hot files so later waves stay disjoint

## Objective
Every shared/hot file is changed exactly once here. After this task: all proto additive changes are in platform-core and re-vendored+generated into consumers; `team-promotion` compiles as an empty service registered in compose + gateway + gitops; migration numbers reserved; e2e/frontend registries stubbed. No business logic yet.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/promotion/v1/promotion.proto        (create)
- platform-core/packages/proto/platform/search/v1/search.proto              (edit — add Facets)
- platform-core/packages/proto/platform/engagement/v1/engagement.proto      (edit — wishlist + review fields/RPCs)
- platform-core/packages/proto/platform/notification/v1/notification.proto  (edit — alert RPCs/enum)
- platform-core/packages/proto/platform/order/v1/order.proto                (edit — voucher_code/discount_amount)
- team-promotion/**                                                          (create — whole new service skeleton)
- team-gateway/internal/upstream/*.go                                        (edit — add Promotion client dial)
- team-gateway/internal/edge/server.go                                       (edit — register promotion handler+path)
- docker-compose.services.yaml                                               (edit — team-promotion block + migrate job)
- AGENTS.md                                                                  (edit — §4 port table: team-promotion :50061)
- team-promotion/FEATURES.yaml                                               (create — planned entries)
- platform-e2e/src/pages/listing_detail_page.py                              (edit — locators for FlashSale/Reviews/WishlistButton/AlertToggle slots)
- platform-e2e/src/pages/checkout_page.py                                    (edit — voucher-input locators)
- platform-gitops/argocd/apps/team-promotion.yaml                           (create)
- platform-gitops/envs/local/values.yaml                                     (edit — image tag)
- team-frontend/src/generated/**                                             (regenerate — Connect-ES stubs)
- team-frontend/src/app/listing/[id]/page.tsx                                (edit — render slots: FlashSale, Reviews, WishlistButton)
- team-frontend/src/app/checkout/page.tsx                                     (edit — render voucher-input slot)
- team-frontend/src/components/nav/**                                         (edit — nav/route entries for vouchers/collections/alerts)
- (re-vendor) team-{promotion,order,search,engagement,notification,gateway}/proto/** + generated/** (regenerate via buf)

## Contracts
All from 00-spec.md §Contracts (promotion.proto full; additive blocks for search/engagement/notification/order). Additive-only: never renumber/remove existing fields.

## Steps
1. Edit protos in platform-core. `buf lint` + `buf breaking` (FILE-level, against main) must pass — run in Docker.
2. Re-vendor `proto/` into each consumer repo, `buf generate` → `generated/`.
3. Scaffold team-promotion by copying team-order skeleton; empty handlers/services/repositories that compile against generated stubs (return Unimplemented is fine); wire `main.go` + `grpcserver` to Register VoucherService + FlashSaleService; `config.go` (+ FeatureFlags, DB, Flipt), `.env.example` (every declared env key), migrations dir with `0001_promotion.up/down.sql` (vouchers, voucher_reservations, flash_sale_campaigns tables).
4. Gateway: add Promotion upstream client + register handler/path in edge/server.go (forwarder bodies come in Wave 2 — leave a stub file owned by Wave 2, do NOT create edge/promotion.go here).
5. docker-compose: team-promotion service block (port 50061) + team-promotion-migrate job + env (DB, FLIPT_ADDR, Kafka). AGENTS.md §4 port row.
6. platform-e2e/FEATURES.yaml: add promotion / search-facets / wishlist-alerts / rich-reviews entries, status: planned.
7. gitops: use skill `scaffold-service` (grpc variant) for team-promotion argocd app + image tag + Vault list.
8. Frontend: register the new/changed services in the gateway client module only (no pages).

## Acceptance criteria
- [ ] `buf lint` + `buf breaking` green; no field renumbered/removed.
- [ ] team-promotion `go build ./...` + `go vet` clean; grpc server boots, health OK.
- [ ] `python scripts/repo_doctor.py` shows no compose/port-table/proto-regen drift.
- [ ] Migration numbers reserved per 00-spec (no collisions downstream).

## Review (gate — different agent)
Route to **contract-boundary-reviewer** (proto + gateway + compose + new service). Require CLEAN.

## Verify
```bash
python scripts/repo_doctor.py
# + in Docker: buf lint && buf breaking && (cd team-promotion && go build ./... && go vet ./...)
```

## Out of scope
- No business logic in team-promotion (Wave 1) — empty/Unimplemented handlers only.
- Do NOT write gateway forwarder bodies or frontend pages (Wave 2).
- Do NOT write feature business logic in order/search/engagement/notification (Wave 1).
- Do NOT create e2e feature files (Wave 3).
