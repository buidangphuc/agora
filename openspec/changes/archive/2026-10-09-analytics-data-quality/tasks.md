## 1. Contract (serial, first)

- [x] 1.1 platform-core: add the RPC and messages (D3) to `analytics.proto`; vendor into team-analytics, team-gateway, team-frontend and regenerate; verify `buf lint`, `buf breaking` (additive) and that each consumer builds

## 2. Code track (one agent per repo, worktree each)

- [x] 2.1 team-analytics: `tracking_ingest_counters` and the sink counting (D1); the report query and status (D2) behind the admin scope; window validation; the three settings; verify DuckDB and service tests for counts, lag (null `ingested_at` excluded), each reason, the window bounds, non-admin denied; then `make check`
- [x] 2.2 team-gateway: forwarder method, `adminProcedures` entry, cockpit `tracking_quality` (D4); verify policy pin test, cockpit tests (section present, null on upstream error), `make check`
- [x] 2.3 team-frontend: tracking quality panel on `/admin/cockpit` with the unavailable state (D4); verify vitest for status/reasons/table and null, `npm run check`

## 3. E2E track (platform-e2e; team-analytics, team-gateway and team-frontend FEATURES.yaml)

- [x] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [x] 3.2 `analytics/tracking_quality_report.feature` (duplicates counted, fresh views, incomplete, window out of range, access); verify against the stack
- [x] 3.3 `ops/cockpit_tracking_quality.feature` (panel shows status and count; unavailable when team-analytics is stopped, destructive); verify both pass
- [x] 3.4 The existing cockpit scenarios stay green

## 4. Review and verify

- [x] 4.1 `contract-boundary-reviewer` (additive proto, analytics owns the report, gateway only shapes) and `auth-scope-reviewer` (admin gate at service and edge); verify no blocking finding
- [x] 4.2 Gate: `openspec validate analytics-data-quality --strict`, `features.py --strict`, `spec_sync.py analytics-data-quality --strict`, `repo_doctor`
- [x] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane; every new flake root-caused

## Evidence (2026-10-09)

- Code: proto a499ce93 and vendoring 043f0eb4 (`buf lint` and `buf breaking` against eda184f7 are clean); analytics 13cc98aa; gateway 1d593dee; frontend f2cbc22a.
- e2e: spec_sync 11/11 (7 new; the 4 modified edge-route-policy scenarios already existed). The adq scenarios ran 6/6 non-destructive twice and 1/1 destructive. Full suite: 449/449 parallel twice.
  - A third parallel run was invalid. It ran while the host's network/DNS was down (agents failed with ENOTFOUND at the same time), took 10 min instead of about 4, and 47 UI scenarios timed out. It was discarded and rerun green.
  - The destructive lane passed 48/49. The one failure ("backend failure is not a silent empty", same window) passed on rerun.
- Defect found on the stack and fixed: the fresh-views check accepted older views and a pre-restart ingest time (test fix).
- Reviews:
  - auth-scope found no blocking issue.
  - The admin gate holds at the service, at the edge and on the cockpit path. There is no PII in the report. The proto copies are byte-identical.
  - Follow-ups:
    - after a crash the counters can undercount, since they are written after the batch commit;
    - the frontend vendors `analytics.proto` but reads only gateway JSON.

## 5. Archive

- [x] 5.1 `openspec archive analytics-data-quality`; verify the folded specs
