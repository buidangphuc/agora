## 1. Code track (one agent per repo, worktree each)

- [x] 1.1 platform-featurestore: dataset registry entry and lock (D1), `als_interactions_v1.sql` (D2), `python -m featurestore dataset` with output + manifest + file SHA (D3); verify unit tests over fixture Parquet (weights per event type, favourite +3 and removal, review ±2, ≤0 dropped, stitched user_key, AS_OF exclusion, manifest SHA equals file SHA, lock drift), `ruff` + `pytest`
- [x] 1.2 platform-recsys: `dataset.py` resolution and refusal (D4), ALS pipeline on the dataset, evaluation on `last_occurred_at`, lineage in `parameters.dataset`, removal of the raw-events ALS path and its settings; verify unit tests (resolution order, empty dir exits 2 naming DATASET_DIR, lineage recorded, weights used as given) and the repo checks
- [x] 1.3 Root compose: recsys job mounts and `DATASET_DIR`, featurestore `dataset` command (D5); verify `docker compose --profile featurestore --profile recsys-train config` (use the repo's actual profile names)

## 2. E2E track (platform-e2e; platform-featurestore and platform-recsys FEATURES.yaml)

- [x] 2.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [x] 2.2 `featurestore/governed_datasets.feature`: weighted row, stitched row, AS_OF exclusion, manifest SHA, lineage, refusal; verify against the stack
- [x] 2.3 Move the existing recsys pipeline scenarios to a dataset fixture (`recsys_job_driver.py`); verify they stay green

## 3. Review and verify

- [x] 3.1 `contract-boundary-reviewer` (recsys reads only the governed dataset, featurestore owns it, no raw-event fallback); verify no blocking finding
- [x] 3.2 Gate: `openspec validate featurestore-datasets --strict`, `features.py --strict`, `spec_sync.py featurestore-datasets --strict`, `repo_doctor`
- [x] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## Evidence (2026-10-09)

- Code: featurestore 36bf0e0f; recsys 2bd1ebd0; compose 72e675c6. Review-driven fix 97edee22: the gitops recsys CronJob set the removed warehouse settings, now `DATASET_DIR` with a fail-closed empty volume, plus stale compose/FEATURES text.
- e2e: spec_sync 6/6. fsd 6/6 plus the recommendations suite (19), with the 6 pipeline scenarios moved to a dataset fixture.
  - Full suite: two parallel runs with one unrelated UI/report failure each (467/468), while a recsys Spark test container ran on the host. Rerun on an idle host: 468/468 twice (`-n 4`). Destructive lane 50/50.
  - Both earlier failures pass when rerun: 26/26.
  - Lane wall time is about 9 min now, because the featurestore scenarios wait for one analytics export cycle (300 s).
- Reviews: contract-boundary found one blocking issue, gitops (fixed). Follow-ups:
  - favourites and reviews older than the window add no weight; this matches the spec's window rule;
  - the e2e fixture duplicates the weights table;
  - the `featurestore-dataset` compose service carries unused Redis env;
  - a cluster job that builds datasets (with a shared volume) is still missing.

## 4. Archive

- [ ] 4.1 `openspec archive featurestore-datasets`; verify `openspec list`
