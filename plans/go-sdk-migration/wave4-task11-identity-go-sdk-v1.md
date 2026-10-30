# [W4-T11] identity-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/identity-go-sdk-v1`)

## Objective
Implement OpenSpec change `identity-go-sdk-v1` test-first per its tasks.md: migrate team-identity to go-sdk v1.0.2, ADR-0016 layout (internal/app, internal/config, internal/rpc, internal/identity, internal/store with embedded migrations and storetest fakes, internal/events, internal/upstream, internal/token, internal/authz), shared make/lint, outbox on the SDK relayer with the 0009-0019 migrations, token and JWKS over `jwtx`, audit writer over `auditx`, no dependency enable flags, no runtime noop or in-memory fallback, `migrate up` subcommand, FEATURES.yaml entries of task 2.1; context design.md Clarifications. Compose and gitops edits belong to deploy-go-sdk-v1, not here.

## Write-set (EXCLUSIVE)
- team-identity/**
- openspec/changes/identity-go-sdk-v1/**

## Verify
- cd team-identity && make proto && make check && go build ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race ./... -count=1
- docker build -t team-identity:go-sdk-check team-identity
- openspec validate identity-go-sdk-v1 --strict
- make -C platform-e2e features-check
