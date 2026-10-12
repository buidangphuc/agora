# feature-run add-ga4-export-import
initiative: ga4-analytics | base: fc50fc6ed32a15d4d6f0e5b9df694377bedabcb8 | worktree: .claude/worktrees/add-ga4-export-import | branch: asdlc/add-ga4-export-import | packet: plans/ga4-analytics/wave1-task2-add-ga4-export-import.md | started: 2026-10-12
## Preflight   validate=pass spec_lint=pass spec_sync=32 uncovered (expected)
## E2E         author=done run=red as expected (18 red: no /ga4import, GetChannelFunnel 404, no ga4_missing; W0 funnel 4 red 404) files=4 rounds=2 (features-check covered_by fix); 14 scenarios no e2e + no VERIFIED BY → spec-clarify pending
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
| wh-ga4 | parity, migration idempotent, export_date, BQ migration failure | approved | 1 | 0cf8e4e4 | PASS | none | approve | - |
| ga4import | fixture day, idempotent, missing table, landing page, empty day, no-op, invalid threshold, 5M budget (34s) | approved | 1 (+test-defect fix) | bd3b8d33 | PASS | none | approve | - |
| attributed-view | matching id, no id, two sessions, outside session, session-end bound, ended_at, missing import reason | approved | 1 | fa2569db | PASS | none | approve (minor: same-ts sessions could duplicate) | - |
| consumer-consent-cols | consent+client id persisted, blanked w/o analytics, legacy = no consent | approved | 1 (+parity test fix, Red-First-Exempt) | 96716fe9 | PASS | none | approve (minor: BQ relies on evolveSchema, not MigrationDDL) | - |
| channel-funnel | split by channel, empty window, invalid window, admin-only, revenue guard, p95 + seller filter | approved | 1 | 7225a2fb | PASS | none (auth per LOCK) | approve (minor: p95 data shape weak; empty txn id) | auth-scope PASS (gateway gate = task gateway-funnel-gate) |
| reconcile | divergence >/= threshold, zero first-party, missing import reason, failure isolated, admin-only reasons | approved (r2: wired + /metrics :9109) | 2 | 6902689a | PASS | none | | | | |
| export-ga4 | Parquet export of ga4_sessions + attributed view | approved | 1 | e01b10fc | PASS | none | approve (privacy note: Parquet copies retention/deletion not declared) | - |
| gitops-ga4 | CronJob no-op unconfigured, Ga4ReconcileDivergence alert, metrics port | approved (to be reworked: no CronJob) | 1 | a02ef954 | n/a manifests (manual red 6→green 10) | none | approve (minor: no BQ creds) | - |
| ga4import-in-consumer | in-process import + internal trigger, CLI thin client, no CronJob, trigger not via gateway | approved | 2 | 5c836308 | PASS | none | approve r2 (r1: env.example, e2e docker exec) | - |
| gateway-funnel-gate | GetChannelFunnel admin at edge (+forwarder) | approved | 1 | 43b2a32d | PASS | none (auth per LOCK) | approve | auth-scope PASS |
## Gates       converge r2 e2e 22/23 (counts-by-channel needs consent-slice gateway → integration converge); FEATURES flipped (features-check pass; spec_sync strict: ga4-consent-contract pass, export-import 1 uncovered = counts-split)
## Escalations
- packet wave1-task2: warehouse write-set widened to team-analytics/internal/warehouse/** (= tasks.md wh-ga4); validate_plan still clean
- human (2026-10-12): add VERIFIED BY lines for unit-only scenarios + not-testable FEATURES entries via spec-clarify after code tasks land, then re-lock
- follow-up (ga4import review, NEEDS_HUMAN at step 5): job writes DuckDB only (Run takes *duckdb.Writer) → prod BigQuery warehouse cannot run it; BigQuerySource untested; config: empty non-string env now falls back to default for ALL settings (call out in PR)
- human (2026-10-12): plan gap — consumer persists ga_client_id + consent cols → new task consumer-consent-cols in this slice (spec-clarify running; re-lock after)
- reconcile r1: option + gauge never wired into cmd/consumer (inert in prod) → packet adds cmd/consumer/main.go; round 2
- NEEDS_HUMAN (privacy): proposal Data classification does not state that exported Parquet (ga4_sessions, tracking_events_attributed with ga_client_id/user_pseudo_id) inherit 13-month retention + deletion
- human (2026-10-12): DuckDB single-writer lock → import runs INSIDE the consumer (daily schedule + internal trigger); CronJob dropped; writer-neutral (BigQuery too). spec-clarify + re-lock, then tasks ga4import-in-consumer + gitops rework + e2e trigger update
- merged into asdlc/ga4-analytics as c92f6c5d (audit clean after adding platform-core/FEATURES.yaml to packet — planned W0 funnel flip); feature-reviewer pass (advisory; counts-split planned)
