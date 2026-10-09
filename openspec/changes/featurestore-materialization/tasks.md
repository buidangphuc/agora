## 1. Code track (one agent per repo, worktree each)

- [x] 1.1 team-analytics: export `tracking_events_resolved`, `engagement_facts` and `order_facts` to Parquet beside the existing file, each atomically; verify export tests for the four files and an atomic replace, then `make check`
- [x] 1.2 platform-featurestore: registry and lock (D2), point-in-time inputs (D3), materialise with offline snapshot, manifest and online writes (D4), parity (D5), `python -m featurestore {materialize,parity}`, Dockerfile (D6), README rewrite; verify unit tests over fixture Parquet files (each feature, the AS_OF exclusion, 7/30-day windows, current-state reduction, manifest contents, parity mismatch exit 3, hash guard) with fakeredis, then `make lint test`
- [x] 1.3 Root compose `featurestore-job` (profile) and volume; platform-core ADR-0015 (D7); verify `docker compose --profile featurestore config`

## 2. E2E track (platform-e2e; team-analytics and platform-featurestore FEATURES.yaml)

- [x] 2.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [x] 2.2 `featurestore/feature_materialization.feature`: resolved export, buyer features, AS_OF exclusion, two snapshots with manifests, online freshness meta, tampered parity; it runs the real job image on the stack network (driver modelled on `recsys_job_driver.py`); verify against the stack

## 3. Review and verify

- [x] 3.1 `contract-boundary-reviewer` (featurestore reads only exports, owns its Redis DB index and volume; analytics owns the warehouse); verify no blocking finding
- [x] 3.2 Gate: `openspec validate featurestore-materialization --strict`, `features.py --strict`, `spec_sync.py featurestore-materialization --strict`, `repo_doctor`
- [x] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## Evidence (2026-10-09)

- Code:
  - team-analytics f23466a1: the four Parquet exports.
  - platform-featurestore b7fc4bb2: the job.
  - platform-featurestore 53531d6f, after the boundary review: the inputs are AS_OF-filtered temp tables, and external access is disabled and locked, so no definition can read Parquet directly. The in-run parity check reads the written snapshot.
  - Compose and ADR-0015 280938f8.
- Spec corrected for a fact the code found: `order_facts` has no buyer column, so per-user order counts left `user_activity@v1`. Follow-up: add `buyer_id` to the order facts, then `user_activity@v2`. The manifest lives under `runs/<as_of>/` (D4).
- e2e:
  - spec_sync 6/6.
  - fsm scenarios 6/6 against the real job image, and 6/6 again on the reviewed image.
  - Full suite 462/462 twice in the parallel lane (`-n 4`), then the destructive lane 50/50.
- Reviews: contract-boundary found no blocking issue. N1 and N2 were fixed in 53531d6f. Follow-ups:
  - parity does not check extra online keys or an empty view;
  - `platform-featurestore/uv.lock` is untracked.

## 4. Archive

- [ ] 4.1 `openspec archive featurestore-materialization`; retire `add-platform-featurestore` (superseded) and `analytics-feature-store` (decision D1 moved features out of team-analytics); verify `openspec list`
