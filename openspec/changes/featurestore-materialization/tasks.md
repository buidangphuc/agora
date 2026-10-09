## 1. Code track (one agent per repo, worktree each)

- [ ] 1.1 team-analytics: export `tracking_events_resolved`, `engagement_facts` and `order_facts` to Parquet beside the existing file, each atomically; verify export tests for the four files and an atomic replace, then `make check`
- [ ] 1.2 platform-featurestore: registry and lock (D2), point-in-time inputs (D3), materialise with offline snapshot, manifest and online writes (D4), parity (D5), `python -m featurestore {materialize,parity}`, Dockerfile (D6), README rewrite; verify unit tests over fixture Parquet files (each feature, the AS_OF exclusion, 7/30-day windows, current-state reduction, manifest contents, parity mismatch exit 3, hash guard) with fakeredis, then `make lint test`
- [ ] 1.3 Root compose `featurestore-job` (profile) and volume; platform-core ADR-0015 (D7); verify `docker compose --profile featurestore config`

## 2. E2E track (platform-e2e; team-analytics and platform-featurestore FEATURES.yaml)

- [ ] 2.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [ ] 2.2 `featurestore/feature_materialization.feature`: resolved export, buyer features, AS_OF exclusion, two snapshots with manifests, online freshness meta, tampered parity; it runs the real job image on the stack network (driver modelled on `recsys_job_driver.py`); verify against the stack

## 3. Review and verify

- [ ] 3.1 `contract-boundary-reviewer` (featurestore reads only exports, owns its Redis DB index and volume; analytics owns the warehouse); verify no blocking finding
- [ ] 3.2 Gate: `openspec validate featurestore-materialization --strict`, `features.py --strict`, `spec_sync.py featurestore-materialization --strict`, `repo_doctor`
- [ ] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## 4. Archive

- [ ] 4.1 `openspec archive featurestore-materialization`; retire `add-platform-featurestore` (superseded) and `analytics-feature-store` (decision D1 moved features out of team-analytics); verify `openspec list`
