## 1. Contract (serial, first)

- [x] 1.1 platform-core: the RPC and messages (D2); vendor into team-analytics, team-gateway, team-frontend; verify `buf lint`, `buf breaking`, builds

## 2. Code track (one agent per repo, worktree each)

- [x] 2.1 team-analytics: the attribution query (D1), the service method (admin, window bounds), `RECS_ATTRIBUTION_WINDOW_HOURS`; verify DuckDB tests (counts per placement/model, attribution inside/outside the window, earliest click only, unattributed purchase, fallback share, empty window) and service tests (access, bounds), `make check`
- [x] 2.2 team-gateway: forwarder and `adminProcedures` (D3); verify the pin test and `make check`

## 3. E2E track (platform-e2e; team-analytics and team-gateway FEATURES.yaml)

- [x] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [x] 3.2 `analytics/recommendation_performance.feature`: the five scenarios through the gateway, using beacons posted with known impression ids, placements and model versions; verify they pass

## 4. Review and verify

- [x] 4.1 `contract-boundary-reviewer` and `auth-scope-reviewer`; verify no blocking finding
- [x] 4.2 Gate: `openspec validate recsys-online-evaluation --strict`, `features.py --strict`, `spec_sync.py recsys-online-evaluation --strict`, `repo_doctor`
- [x] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## Evidence (2026-10-09)

- Code: proto e50f1f60, vendoring 3feca3a7 (`buf breaking` against e36ca4a1 clean); team-analytics 9284714c; team-gateway 29a7687c. Review fix 341eac33: conversions are no longer attributed through the shared empty anonymous key `anon:`. Its regression test fails without the fix.
- Spec: the scenarios name test-owned placements (0afc2735), because shared placements carry other tests' traffic. The design notes that attribution is client-reported.
- e2e: spec_sync 9/9 (5 new plus 4 existing edge-route-policy). roe 5/5 on the first run. Full suite 474/474 twice in the parallel lane (`-n 4`), then destructive lane 63/63. After the review fix, the analytics scenarios (roe, adq, tii) ran 22/22.
- Reviews: contract-boundary found no blocking issue. Follow-ups:
  - Attribution is forgeable through client beacons. This is acceptable for a descriptive admin report; purchases could later come from order facts.
  - A click is not checked against the impression's listings.
  - A reused impression id collapses to one placement and model.
  - Conversion windows near the report's end are truncated.

## 5. Archive

- [x] 5.1 `openspec archive recsys-online-evaluation`; retire `recommendations-end-to-end` (its remaining requirements are covered by changes 5–8); verify `openspec list`
