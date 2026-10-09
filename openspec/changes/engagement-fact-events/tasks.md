## 1. Contract (serial, first)

- [ ] 1.1 platform-core: `platform/engagement/v1/events.proto` (D2); vendor into team-engagement and team-analytics and regenerate; verify `buf lint`, `buf breaking` (additive), both build

## 2. Code track (one agent per repo, worktree each)

- [ ] 2.1 team-engagement: migration 0010, the transactional outbox writes in the five methods (no-op skips), the relayer, the settings (D1); verify repository tests with a throwaway Postgres (fact written with the change, none on a no-op, a rolled-back transaction writes none), relayer order test, `make check`
- [ ] 2.2 team-analytics: engagement consumer, `engagement_facts`, the views, the DLQ (D3); verify DuckDB tests for each fact, idempotency, the views and the DLQ path, `make check`
- [ ] 2.3 Topics and config: root compose `redpanda-init`, compose env for both services, gitops Redpanda topics and values (D4); verify `docker compose config`, `helm template`

## 3. E2E track (platform-e2e; team-engagement and team-analytics FEATURES.yaml)

- [ ] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [ ] 3.2 `engagement/engagement_facts.feature`: favourite, removal, repeat, review rating without text, Kafka-down follow (destructive); verify against the stack
- [ ] 3.3 `analytics/engagement_facts_warehouse.feature`: current favourites, current follows, malformed record dead-lettered; verify they pass

## 4. Review and verify

- [ ] 4.1 `contract-boundary-reviewer` (outbox, broker, ownership, additive proto) and `auth-scope-reviewer` (the envelope principal is the caller; no review text or PII in payloads); verify no blocking finding
- [ ] 4.2 Gate: `openspec validate engagement-fact-events --strict`, `features.py --strict`, `spec_sync.py engagement-fact-events --strict`, `repo_doctor`
- [ ] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane; every new flake root-caused

## 5. Archive

- [ ] 5.1 `openspec archive engagement-fact-events`; retire `tracking-event-platform` (its non-legal requirements are covered by changes 1–3; the consent/retention/erasure part stays in `plans/ai-first/DECISIONS.md` for the legal change); verify `openspec list`
