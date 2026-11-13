# [W0-T1] monetization-contract

## Role
feature-run worker (change `openspec/changes/monetization-contract`)

## Objective
Implement OpenSpec change `monetization-contract` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- Additive proto changes in platform-core: `OrderPaidEvent` fields 9-12, wallet types 4-7, `GetOrderSettlement`, `ChargeSellerFee`, plan term and subscription RPCs, `PAUSED_BUDGET`, sponsored slot/click/campaign RPCs, with documented comments.
- Refresh vendored copies byte-for-byte in the 14 order, 14 payment and 7 promotion consumers; regenerate fe-platform api-client stubs (order, payment) and the vendored api-client tarballs.
- Contract tests: three `monetization_compat_test.go` files and `monetization_consumer_copies_test.sh` (35 pairs).
- depends_on: none
- base: `asdlc/monetization`
- hot file: contract slice: sequential and alone; every consumer slice bases on it after merge

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/order/v1/**
- platform-core/packages/proto/platform/payment/v1/**
- platform-core/packages/proto/platform/promotion/v1/**
- platform-core/packages/proto/monetization_consumer_copies_test.sh
- platform-core/packages/proto/Makefile
- platform-core/FEATURES.yaml
- platform-core/README.md
- team-*/proto/platform/order/v1/order.proto
- team-*/proto/platform/payment/v1/payment.proto
- team-*/proto/platform/promotion/v1/promotion.proto
- fe-platform/packages/api-client/src/generated/platform/order/v1/**
- fe-platform/packages/api-client/src/generated/platform/payment/v1/**
- fe-platform/.changeset/**
- team-frontend*/vendor/fe-platform-api-client.tgz
- team-frontend*/package-lock.json

## Verify
- `cd platform-core/packages/proto && go test ./platform/order/v1/... ./platform/payment/v1/... ./platform/promotion/v1/... && bash monetization_consumer_copies_test.sh`
- `make -C platform-core proto` (buf lint + breaking against origin/main)
- `make -C platform-core check`
- `openspec validate monetization-contract --strict`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . monetization-contract`
