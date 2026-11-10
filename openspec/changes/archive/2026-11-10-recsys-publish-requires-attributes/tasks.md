## 1. Code track: platform-recsys

- [x] 1.1 Tests first, verify red: `tests/test_candidate_attributes.py` `test_missing_snapshot_refuses_before_training`, `test_corrupt_snapshot_refuses_before_training`, `test_stale_snapshot_still_trains`; delete `test_missing_snapshot_publishes_without_attributes` and `test_corrupt_snapshot_publishes_without_attributes`; any other unit test that trains without a snapshot gets one from a shared fixture
- [x] 1.2 `recsys/pipeline.py` (and the run entry in `recsys/__main__.py`): resolve the snapshot before the dataset read and training, exit 2 with `attributes=missing` when it is missing or unreadable; pass the loaded map to `publish_generation` instead of loading it there (Missing snapshot refuses the run, Corrupt snapshot refuses the run, A stale snapshot still trains)
- [x] 1.3 `recsys/load/attributes.py` docstring and README: a missing snapshot refuses the run; update `platform-recsys/FEATURES.yaml` entries of the two removed scenarios

## 2. E2E track: platform-e2e

- [x] 2.1 `tests/e2e/flows/recsys_job_driver.py`: default `item_attributes@v2` snapshot (every fixture listing published) when a run has no `@item_attributes`; `"@item_attributes": null` writes none
- [x] 2.2 `tests/e2e/features/recommendations/candidate_attributes.feature`: scenario `A refused run keeps serving the previous generation`; FEATURES.yaml entry via the e2e-author
- [x] 2.3 Rerun on the main env after promote: the files listed in proposal.md "Why" plus `test_candidate_attributes`, `test_fir_recsys_item_attributes`, `test_mlr_ml_recsys`, `test_recsys_auto_train`, `test_rgp_generation_publish`, `test_rgt_gbdt_ranker`, `test_fsd_governed_datasets`

## 3. Checks

- [x] 3.1 `openspec validate recsys-publish-requires-attributes --strict`, `spec_lint.py`, platform-recsys and platform-e2e CI checks, pre-commit-ci
