# [W1-T2] domain-consumer-key-redaction

## Role
feature-run worker (change `openspec/changes/domain-consumer-key-redaction`)

## Objective
Implement OpenSpec change `domain-consumer-key-redaction` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- team-domain consumer logs key_fp (first 12 hex of SHA-256) instead of the raw Kafka key at the three processRecord log sites; Kafka key, retry and DLQ behaviour unchanged.
- depends_on: none

## Write-set (EXCLUSIVE)
- team-domain/internal/consumer/consumer.go
- team-domain/internal/consumer/consumer_log_test.go
- team-domain/README.md
- team-domain/FEATURES.yaml
- openspec/changes/domain-consumer-key-redaction/**

## Read-only dependencies
- openspec/changes/domain-consumer-key-redaction/** is frozen by the LOCK.md digest (only task ticks change)
- team-domain/internal/consumer/identity_test.go (fakeDLQ to reuse)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-domain && make proto && go vet ./... && go test ./internal/consumer/... -race && go test ./... -race); $PY platform-e2e/scripts/spec_sync.py domain-consumer-key-redaction --strict  (e2e: none, unit-verified log content)
