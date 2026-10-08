## 1. Contract (serial, first)

- [ ] 1.1 platform-core: add the RPC and messages (D3) to `analytics.proto`; vendor into team-analytics, team-gateway, team-frontend and regenerate; verify `buf lint`, `buf breaking` (additive) and that each consumer builds

## 2. Code track (one agent per repo, worktree each)

- [ ] 2.1 team-analytics: `tracking_ingest_counters` and the sink counting (D1); the report query and status (D2) behind the admin scope; window validation; the three settings; verify DuckDB and service tests for counts, lag (null `ingested_at` excluded), each reason, the window bounds, non-admin denied; then `make check`
- [ ] 2.2 team-gateway: forwarder method, `adminProcedures` entry, cockpit `tracking_quality` (D4); verify policy pin test, cockpit tests (section present, null on upstream error), `make check`
- [ ] 2.3 team-frontend: tracking quality panel on `/admin/cockpit` with the unavailable state (D4); verify vitest for status/reasons/table and null, `npm run check`

## 3. E2E track (platform-e2e; team-analytics, team-gateway and team-frontend FEATURES.yaml)

- [ ] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [ ] 3.2 `analytics/tracking_quality_report.feature` (duplicates counted, fresh views, incomplete, window out of range, access); verify against the stack
- [ ] 3.3 `ops/cockpit_tracking_quality.feature` (panel shows status and count; unavailable when team-analytics is stopped, destructive); verify both pass
- [ ] 3.4 The existing cockpit scenarios stay green

## 4. Review and verify

- [ ] 4.1 `contract-boundary-reviewer` (additive proto, analytics owns the report, gateway only shapes) and `auth-scope-reviewer` (admin gate at service and edge); verify no blocking finding
- [ ] 4.2 Gate: `openspec validate analytics-data-quality --strict`, `features.py --strict`, `spec_sync.py analytics-data-quality --strict`, `repo_doctor`
- [ ] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane; every new flake root-caused

## 5. Archive

- [ ] 5.1 `openspec archive analytics-data-quality`; verify the folded specs
