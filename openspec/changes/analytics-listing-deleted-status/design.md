## Clarifications

- 2026-11-12. Question: what must "A deleted listing is not recommended end to end" observe? Answer: besides the recommendation response, the item point of the deleted listing in the serving collection carries `visible=false`.
- 2026-11-12. Question: which test verifies "The v2 definition and lock are unchanged"? Answer: `test_committed_lock_matches_registry` in `platform-featurestore/tests/test_materialize.py`.
