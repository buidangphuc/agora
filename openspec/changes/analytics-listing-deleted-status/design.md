## Clarifications

- 2026-11-12. Question: what must "A deleted listing is not recommended end to end" observe? Answer: besides the recommendation response, the item point of the deleted listing in the serving collection carries `visible=false`.
- 2026-11-12. Question: which test verifies "The v2 definition and lock are unchanged"? Answer: `test_committed_lock_matches_registry` in `platform-featurestore/tests/test_materialize.py`.
- 2026-11-12 (Bùi Đăng Phúc): traceability-only edits so spec_sync --strict passes — renamed scenarios "A deleted listing keeps its row with status DELETED" to "A deleted listing is exported as DELETED" and "Replay re-derives DELETED for a listing deleted before this change" to "Replay re-derives DELETED for a listing deleted earlier"; tasks.md citations updated; no GIVEN/WHEN/THEN text changed.
