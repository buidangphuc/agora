## 1. Code track (one agent per repo, worktree each)

- [ ] 1.1 platform-featurestore: dataset registry entry and lock (D1), `als_interactions_v1.sql` (D2), `python -m featurestore dataset` with output + manifest + file SHA (D3); verify unit tests over fixture Parquet (weights per event type, favourite +3 and removal, review ±2, ≤0 dropped, stitched user_key, AS_OF exclusion, manifest SHA equals file SHA, lock drift), `ruff` + `pytest`
- [ ] 1.2 platform-recsys: `dataset.py` resolution and refusal (D4), ALS pipeline on the dataset, evaluation on `last_occurred_at`, lineage in `parameters.dataset`, removal of the raw-events ALS path and its settings; verify unit tests (resolution order, empty dir exits 2 naming DATASET_DIR, lineage recorded, weights used as given) and the repo checks
- [ ] 1.3 Root compose: recsys job mounts and `DATASET_DIR`, featurestore `dataset` command (D5); verify `docker compose --profile featurestore --profile recsys-train config` (use the repo's actual profile names)

## 2. E2E track (platform-e2e; platform-featurestore and platform-recsys FEATURES.yaml)

- [ ] 2.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [ ] 2.2 `featurestore/governed_datasets.feature`: weighted row, stitched row, AS_OF exclusion, manifest SHA, lineage, refusal; verify against the stack
- [ ] 2.3 Move the existing recsys pipeline scenarios to a dataset fixture (`recsys_job_driver.py`); verify they stay green

## 3. Review and verify

- [ ] 3.1 `contract-boundary-reviewer` (recsys reads only the governed dataset, featurestore owns it, no raw-event fallback); verify no blocking finding
- [ ] 3.2 Gate: `openspec validate featurestore-datasets --strict`, `features.py --strict`, `spec_sync.py featurestore-datasets --strict`, `repo_doctor`
- [ ] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## 4. Archive

- [ ] 4.1 `openspec archive featurestore-datasets`; verify `openspec list`
