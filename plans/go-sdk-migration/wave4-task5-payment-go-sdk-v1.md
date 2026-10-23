# [W4-T5] payment-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/payment-go-sdk-v1`)

## Objective
Implement OpenSpec change `payment-go-sdk-v1` test-first per its tasks.md: migrate team-payment to go-sdk v1.0.2, ADR-0016 layout (internal/config, internal/store, internal/payment, internal/upstream, internal/rpc, internal/settlement, internal/events, internal/app), shared make/lint, no dependency enable flags, no in-memory runtime fallback, mock payments through `MOCK_DATA=payment-provider`, outbox on the SDK, `migrate up` subcommand; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-payment/**
- openspec/changes/payment-go-sdk-v1/**
- team-payment/internal/app/app.go (widened at build: task 1.8)
- team-payment/internal/payment/service.go (widened at build: task 1.8)
- team-payment/internal/payment/settlement.go (widened at build: task 1.8)
- team-payment/internal/rpc/wallet_ledger.go (widened at build: task 1.8)
- team-payment/internal/settlement/handler.go (widened at build: task 1.8)
- team-payment/internal/store/ledger.go (widened at build: task 1.8)
- team-payment/internal/store/fake/ledger.go (widened at build: task 1.8)
- team-payment/internal/store/fake/settlement.go (widened at build: task 1.8)
- team-payment/internal/payment/payment_test.go (widened at build: task 1.8)
- team-payment/internal/payment/refund_test.go (widened at build: task 1.8)
- team-payment/internal/payment/service_test.go (widened at build: task 1.8)
- team-payment/internal/payment/unpaid_cancel_test.go (widened at build: task 1.8)
- team-payment/internal/payment/helpers_test.go (widened at build: task 1.8)
- team-payment/internal/store/payment_test.go (widened at build: task 1.8)

## Verify
- cd team-payment && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-payment:sdk team-payment
- openspec validate payment-go-sdk-v1 --strict
- make -C platform-e2e features-check
