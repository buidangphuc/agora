# [W0-T1] seller-centre-contract

## Role
feature-run worker (change `openspec/changes/archive/2026-11-17-seller-centre-contract`)

## Objective
Implement OpenSpec change `seller-centre-contract` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- Additive proto contract: 4 seller promotion RPCs + `FlashSaleCampaign.seller_id = 11` (promotion.proto); `AddBundleToCart`, bundle fields on `CartItem`/`Cart`/`OrderItem`, paged `ListSellerOrders` + `OrderStatusCount` (order.proto); `ListMyListingsRequest.statuses/query` and storefront rule comments (listing.proto).
- Compat Go tests first (red); copy the three protos byte-identical into every consumer; `sellercentre_consumer_copies_test.sh` with drift/missing self-checks; regenerate all languages.
- depends_on: none
- base: `asdlc/seller-centre`

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/promotion/v1/**
- platform-core/packages/proto/platform/order/v1/**
- platform-core/packages/proto/platform/listing/v1/**
- platform-core/packages/proto/sellercentre_consumer_copies_test.sh
- platform-core/packages/proto/Makefile
- platform-core/FEATURES.yaml
- platform-core/README.md
- team-*/proto/platform/promotion/v1/promotion.proto
- team-*/proto/platform/order/v1/order.proto
- team-*/proto/platform/listing/v1/listing.proto
- team-frontend*/package-lock.json
- team-frontend*/vendor/fe-platform-api-client.tgz
- fe-platform/packages/api-client/src/generated/**
- fe-platform/.changeset/**
- team-*/README.md
- platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go (widened at build: P1, pinned VoucherService RPC list broken by the new ListMyShopVouchers/DeactivateMyShopVoucher RPCs)

## Verify
- `cd platform-core/packages/proto && go test ./platform/promotion/v1/... ./platform/order/v1/... ./platform/listing/v1/... && bash sellercentre_consumer_copies_test.sh`
- `make -C platform-core proto` (buf lint + breaking against origin/main; mutated-copy check per task 3.1)
- `make -C platform-core check`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . seller-centre-contract` and `openspec validate seller-centre-contract --strict`
