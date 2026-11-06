# [W0-T1] q7-referral-welcome-contract

## Role
feature-run worker (change `openspec/changes/q7-referral-welcome-contract`)

## Objective
Implement OpenSpec change `q7-referral-welcome-contract` test-first per its tasks.md (scenario ids there); context in plans/q7-q37-referral-welcome/00-spec.md (Locked + Clarifications C1-C6) and the change's design.md.
- Additive contract: AuthService.GetMyAccount, RegisterRequest.referral_code, UserRegistered; referral enums, events and reward/summary fields; VoucherKind, Voucher/Create/Update user-bound fields, VoucherService.ListMyVouchers; OrderService.HasCompletedOrder (internal). Byte-identical copies into the consumer repos.
- depends_on: none

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/identity/v1/*.proto
- platform-core/packages/proto/platform/referral/v1/*.proto
- platform-core/packages/proto/platform/promotion/v1/*.proto
- platform-core/packages/proto/platform/order/v1/*.proto
- platform-core/packages/proto/platform/referral/v1/*_test.go
- platform-core/packages/proto/q7_consumer_copies_test.sh
- platform-core/gen/**
- platform-core/FEATURES.yaml
- team-*/proto/platform/identity/v1/identity.proto
- team-*/proto/platform/referral/v1/referral.proto
- team-*/proto/platform/promotion/v1/promotion.proto
- team-*/proto/platform/order/v1/order.proto
- team-referral/proto/platform/payment/v1/payment.proto
- team-referral/proto/platform/events/v1/*.proto
- team-promotion/proto/platform/payment/v1/payment.proto

## Verify
- `cd platform-core/packages/proto/platform/referral/v1 && go test ./...`
- `bash platform-core/packages/proto/q7_consumer_copies_test.sh`
- `make -C platform-core lint-proto breaking`
- `make -C platform-core proto`
