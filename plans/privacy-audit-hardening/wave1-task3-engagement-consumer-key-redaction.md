# [W1-T3] engagement-consumer-key-redaction

## Role
feature-run worker (change `openspec/changes/engagement-consumer-key-redaction`)

## Objective
Implement OpenSpec change `engagement-consumer-key-redaction` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- team-engagement consumer logs key_fp (same format as team-domain) instead of the raw Kafka key; DLQ and handler still receive the raw key.
- depends_on: none

## Write-set (EXCLUSIVE)
- team-engagement/internal/consumer/consumer.go
- team-engagement/internal/consumer/consumer_log_test.go
- team-engagement/internal/consumer/*_test.go
- team-engagement/README.md
- team-engagement/FEATURES.yaml
- platform-e2e/tests/e2e/features/engagement/consumer_key_redaction.feature
- platform-e2e/tests/e2e/step_definitions/test_engagement_consumer_key_redaction.py
- openspec/changes/engagement-consumer-key-redaction/**

## Read-only dependencies
- openspec/changes/engagement-consumer-key-redaction/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/step_definitions/test_efe_facts.py (Kafka helpers to reuse)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-engagement && make proto && go vet ./... && go test ./... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_engagement_consumer_key_redaction.py); $PY platform-e2e/scripts/spec_sync.py engagement-consumer-key-redaction --strict
