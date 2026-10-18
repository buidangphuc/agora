# [W2-T1] engagement-review-hidden-event

## Role
feature-run worker (change `openspec/changes/engagement-review-hidden-event`)

## Objective
Implement OpenSpec change `engagement-review-hidden-event` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- team-engagement HideReview enqueues one ReviewHidden outbox fact atomically with an effective hide (none on re-hide, unknown review, audit failure or non-admin).
- depends_on: pah-contract, analytics-review-hidden, engagement-consumer-key-redaction

## Write-set (EXCLUSIVE)
- team-engagement/internal/repository/review.go
- team-engagement/internal/repository/review_hide_fact_test.go
- team-engagement/internal/service/review_test.go
- team-engagement/internal/handler/reviews_hide_test.go
- team-engagement/README.md
- team-engagement/FEATURES.yaml
- platform-e2e/tests/e2e/features/engagement/review_hidden_event.feature
- platform-e2e/tests/e2e/step_definitions/test_review_hidden_event.py
- openspec/changes/engagement-review-hidden-event/**
- team-engagement/internal/docs/readme_test.go # widened at build: task 3

## Read-only dependencies
- openspec/changes/engagement-review-hidden-event/** is frozen by the LOCK.md digest (only task ticks change)
- team-engagement/proto/platform/engagement/v1/events.proto (copied by pah-contract)
- platform-e2e/tests/e2e/step_definitions/test_efe_facts.py, review_integrity_steps.py (reuse)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-engagement && make proto && go vet ./... && go test ./... -race) with TEST_DATABASE_URL on a throwaway postgres; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_review_hidden_event.py) (needs team-analytics from analytics-review-hidden for the analytics scenario); $PY platform-e2e/scripts/spec_sync.py engagement-review-hidden-event --strict
