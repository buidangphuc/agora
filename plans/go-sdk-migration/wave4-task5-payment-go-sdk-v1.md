# [W4-T5] payment-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/payment-go-sdk-v1`)

## Objective
Implement OpenSpec change `payment-go-sdk-v1` test-first per its tasks.md: migrate team-payment to go-sdk v1.0.2, ADR-0016 layout (internal/config, internal/store, internal/payment, internal/upstream, internal/rpc, internal/settlement, internal/events, internal/app), shared make/lint, no dependency enable flags, no in-memory runtime fallback, mock payments through `MOCK_DATA=payment-provider`, outbox on the SDK, `migrate up` subcommand; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-payment/**
- openspec/changes/payment-go-sdk-v1/**
- team-payment/internal/payment/wallet_ledger.go (widened at build: task 1.5)
- team-payment/internal/payment/wallet_ledger_test.go (widened at build: task 1.5)

## Verify
- cd team-payment && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-payment:sdk team-payment
- openspec validate payment-go-sdk-v1 --strict
- make -C platform-e2e features-check
