## 1. Code track (one agent per repo, worktree each)

- [x] 1.1 platform-recsys: generation keys + atomic pointer switch (D1), generation collections + aliases + legacy migration + retention (D2), structural gate (D3), `rollback` command (D4), `RECS_WRITE_LEGACY_KEYS` shim; verify unit tests (pointer swap atomic via Lua with fakeredis, retention keeps two, gate each check incl. NaN, rollback with/without previous, alias moves with an in-memory Qdrant client) and the repo checks incl. the Spark suite in a container
- [x] 1.2 team-ai: pointer-aware `RecommendationCache` with 5 s memo and fallback (D5); verify unit tests (pointer set → gen keys and model_version, absent → legacy keys, memo expiry), `make check` + `make test`

## 2. E2E track (platform-e2e; platform-recsys and team-ai FEATURES.yaml)

- [x] 2.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [x] 2.2 `recommendations/generation_publish.feature`: second promotion keeps previous, third drops oldest, rejected candidate leaves serving, one-size-fits-all rejected, rollback restores, rollback refused, serving follows within 10 s; run the real recsys image on dataset fixtures (`recsys_job_driver.py`) and read recommendations through the gateway; mark scenarios that rewrite the shared serving pointer `@destructive` (they change what every other recommendation scenario sees); verify they pass
- [x] 2.3 The existing recommendation scenarios stay green after the generation switch

## 3. Review and verify

- [x] 3.1 `contract-boundary-reviewer` (recsys owns its keys/collections; team-ai reads only; no proto change); verify no blocking finding
- [x] 3.2 Gate: `openspec validate recsys-generation-publish --strict`, `features.py --strict`, `spec_sync.py recsys-generation-publish --strict`, `repo_doctor`
- [x] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## Evidence (2026-10-09)

- **Code.**
  - recsys a8f6e699, team-ai 4cd5645f.
  - Fixes found on the stack: 2242b0de (the legacy collection is copied at its own vector size).
  - Fixes from the boundary review: 12a17ce2 (the champion moves only after a successful publish), a6f22305 (compare-and-set rollback that converges after a crash), 1db57039 (both aliases move in one call), 455a5a59.
  - Test isolation: team-ai 65bc2ff1 (guard tests no longer need a local .env).
- **e2e.**
  - spec_sync 7/7, with rgp 7/7 run twice, including on the reviewed image.
  - Full suite: 468/468 twice in the parallel lane (`-n 4`), then the destructive lane 57/57.
- **e2e isolation defects, fixed:**
  - 63e3e71a: ai-auth now seeds the serving generation.
  - 41dc0d90: the structural gate stays out of the metric-gate fixtures, and the per-worker alias and generation collections are cleaned.
  - 1baa1971 and the follow-up teardown assertion: the scenarios had left a fixture model serving, which blanked the homepage row. Teardown now republishes a real generation through the production path and fails loudly if it is not promoted.
- **Reviews.** contract-boundary found no blocking issue. Findings 1–3 and 6 are fixed. Follow-ups:
  - Qdrant serves generation N+1 a few milliseconds before the Redis pointer moves.
  - The one-time legacy migration has a brief 404 gap.
  - Rollback does not restore the `RECS_WRITE_LEGACY_KEYS` shim keys.
  - Retention scans the whole `recs:v1:gen:*` keyspace.

## 4. Archive

- [x] 4.1 `openspec archive recsys-generation-publish`; verify `openspec list`
