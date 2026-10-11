## 1. Contract (serial, first)

- [x] 1.1 platform-core: `platform/engagement/v1/events.proto` (D2); vendor into team-engagement and team-analytics and regenerate; verify `buf lint`, `buf breaking` (additive), both build

## 2. Code track (one agent per repo, worktree each)

- [x] 2.1 team-engagement: migration 0010, the transactional outbox writes in the five methods (no-op skips), the relayer, the settings (D1); verify repository tests with a throwaway Postgres (fact written with the change, none on a no-op, a rolled-back transaction writes none), relayer order test, `make check`
- [x] 2.2 team-analytics: engagement consumer, `engagement_facts`, the views, the DLQ (D3); verify DuckDB tests for each fact, idempotency, the views and the DLQ path, `make check`
- [x] 2.3 Topics and config: root compose `redpanda-init`, compose env for both services, gitops Redpanda topics and values (D4); verify `docker compose config`, `helm template`

## 3. E2E track (platform-e2e; team-engagement and team-analytics FEATURES.yaml)

- [x] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [x] 3.2 `engagement/engagement_facts.feature`: favourite, removal, repeat, review rating without text, Kafka-down follow (destructive); verify against the stack
- [x] 3.3 `analytics/engagement_facts_warehouse.feature`: current favourites, current follows, malformed record dead-lettered; verify they pass

## 4. Review and verify

- [x] 4.1 `contract-boundary-reviewer` (outbox, broker, ownership, additive proto) and `auth-scope-reviewer` (the envelope principal is the caller; no review text or PII in payloads); verify no blocking finding
- [x] 4.2 Gate: `openspec validate engagement-fact-events --strict`, `features.py --strict`, `spec_sync.py engagement-fact-events --strict`, `repo_doctor`
- [x] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane; every new flake root-caused

## Evidence (2026-10-09)

- Code: proto 45646c65, vendoring 6f1d440e (`buf breaking` against ad5f35b7 is clean); team-engagement b6ad395a; team-analytics 2af3ac2a; topics and config 825cbb46.
- Review-driven fixes:
  - 84710d87: the analytics engagement env had landed on the team-gateway block in compose.
  - 818065cb: a warning when the engagement consumer cannot start on a non-DuckDB driver.
- e2e: spec_sync 8/8. efe scenarios 7/7 and 1/1 (Redpanda stopped and started again). Full suite 456/456 twice (`-n 4`), then the destructive lane 50/50. efe 7/7 again after the compose env fix.
- Reviews: contract-boundary found one blocking issue (the compose env placement, fixed). Follow-ups:
  - outbox `seq` is assigned at insert, not commit, so two concurrent writers can relay out of order across keys; the same shape exists in team-order;
  - the relayer holds a DB transaction across the publish, which can resend after a failed commit; the consumer dedupes this;
  - the review fact's seller can be empty when the projection lags.

## 5. Archive

- [x] 5.1 `openspec archive engagement-fact-events`; retire `tracking-event-platform` (its non-legal requirements are covered by changes 1–3; the consent/retention/erasure part stays in `plans/ai-first/DECISIONS.md` for the legal change); verify `openspec list`
