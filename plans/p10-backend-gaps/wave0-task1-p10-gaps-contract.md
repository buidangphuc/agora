# [W0-T1] p10-gaps-contract

## Role
feature-run worker (change `openspec/changes/p10-gaps-contract`)

## Objective
Implement OpenSpec change `p10-gaps-contract` test-first per its tasks.md; context in plans/p10-backend-gaps/00-spec.md (Locked + Clarifications) and the change's design.md.
- Add the voucher/campaign admin RPCs, CampaignStatus, hand-off RPCs, BecomeSeller and UserRoleGranted to the promotion/identity protos, extend the audit action catalogue to 24, vendor the copies into every consumer found by glob and regenerate.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/promotion/v1/promotion.proto
- platform-core/packages/proto/platform/promotion/v1/gaps_contract_test.go
- platform-core/packages/proto/platform/identity/v1/identity.proto
- platform-core/packages/proto/platform/identity/v1/gaps_contract_test.go
- platform-core/packages/proto/platform/audit/v1/audit.proto
- platform-core/packages/proto/gaps_contract_docs_test.go
- platform-core/packages/proto/gaps_consumer_copies_test.sh
- platform-core/packages/proto/gen/**
- team-*/proto/platform/promotion/v1/*.proto
- team-*/proto/platform/identity/v1/*.proto
- team-*/proto/platform/audit/v1/*.proto
- team-*/generated/**
- team-frontend*/src/generated/**
- openspec/changes/p10-gaps-contract/**

## Read-only dependencies
- platform-core/packages/proto/consumer_copies_test.sh (pattern)
- platform-core/packages/proto/platform/audit/v1/wire_gate_test.go (must stay green)
- plans/p10-backend-gaps/00-spec.md, LOCK.md

## Verify
PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; run every check through the gate-digest skill (scripts/asdlc/gate_digest.py).
- (cd platform-core/packages/proto && go test ./...)
- bash platform-core/packages/proto/gaps_consumer_copies_test.sh
- make -C platform-core check
- $PY platform-e2e/scripts/spec_sync.py p10-gaps-contract --strict
- e2e: none owed by this slice (tasks.md 2.1)
