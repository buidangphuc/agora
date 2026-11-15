# [W7-T1] content-user-erasure

## Role
feature-run worker (change `openspec/changes/content-user-erasure`)

## Objective
Implement OpenSpec change `content-user-erasure` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-engagement, team-chat, team-notification: `Erase(ctx, userID)` store method in one tx each and a `UserErased` consumer (groups `team-engagement-erasure`, `team-chat-erasure`, `team-notification-erasure`; DLQs `identity.events.{engagement,chat,notification}.dlq`), hashed-id logging.
- Compose: three DLQ topics + topic/group env (additive edit).
- depends_on: trust-legal-contract (wave 0)
- base: `asdlc/trust-legal` after wave 6
- hot file: `docker-compose.services.yaml` (only compose writer in this wave)

## Write-set (EXCLUSIVE)
- team-engagement/**
- team-chat/**
- team-notification/**
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/engagement/user_erasure.feature
- platform-e2e/tests/e2e/features/chat/user_erasure.feature
- platform-e2e/tests/e2e/features/notification/user_erasure.feature
- platform-e2e/tests/e2e/step_definitions/test_engagement_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/test_chat_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/test_notification_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/content_erasure_steps.py
- widened (P1):
  - openspec/changes/content-user-erasure/specs/chat-user-erasure/spec.md — the slice's own OpenSpec folder; only VERIFIED BY citations renamed to the real test names (no requirement or scenario wording change)
  - openspec/changes/content-user-erasure/specs/engagement-user-erasure/spec.md — the slice's own OpenSpec folder; only VERIFIED BY citations renamed to the real test names (no requirement or scenario wording change)
  - openspec/changes/content-user-erasure/specs/notification-user-erasure/spec.md — the slice's own OpenSpec folder; only VERIFIED BY citations renamed to the real test names (no requirement or scenario wording change)
  - platform-e2e/tests/e2e/support/erasure_events_support.py — e2e support file of this change; adds engagement/chat/notification consumer group and DLQ constants

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-engagement --vet` (same for team-chat, team-notification)
- `openspec validate content-user-erasure --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: engagement|chat|notification/user_erasure.feature; DLQs empty after a normal erasure
