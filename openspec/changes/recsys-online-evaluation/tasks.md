## 1. Contract (serial, first)

- [ ] 1.1 platform-core: the RPC and messages (D2); vendor into team-analytics, team-gateway, team-frontend; verify `buf lint`, `buf breaking`, builds

## 2. Code track (one agent per repo, worktree each)

- [ ] 2.1 team-analytics: the attribution query (D1), the service method (admin, window bounds), `RECS_ATTRIBUTION_WINDOW_HOURS`; verify DuckDB tests (counts per placement/model, attribution inside/outside the window, earliest click only, unattributed purchase, fallback share, empty window) and service tests (access, bounds), `make check`
- [ ] 2.2 team-gateway: forwarder and `adminProcedures` (D3); verify the pin test and `make check`

## 3. E2E track (platform-e2e; team-analytics and team-gateway FEATURES.yaml)

- [ ] 3.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [ ] 3.2 `analytics/recommendation_performance.feature`: the five scenarios through the gateway, using beacons posted with known impression ids, placements and model versions; verify they pass

## 4. Review and verify

- [ ] 4.1 `contract-boundary-reviewer` and `auth-scope-reviewer`; verify no blocking finding
- [ ] 4.2 Gate: `openspec validate recsys-online-evaluation --strict`, `features.py --strict`, `spec_sync.py recsys-online-evaluation --strict`, `repo_doctor`
- [ ] 4.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## 5. Archive

- [ ] 5.1 `openspec archive recsys-online-evaluation`; retire `recommendations-end-to-end` (its remaining requirements are covered by changes 5–8); verify `openspec list`
