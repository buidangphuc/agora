## 1. Code track (one agent per repo, worktree each)

- [ ] 1.1 platform-recsys: generation keys + atomic pointer switch (D1), generation collections + aliases + legacy migration + retention (D2), structural gate (D3), `rollback` command (D4), `RECS_WRITE_LEGACY_KEYS` shim; verify unit tests (pointer swap atomic via Lua with fakeredis, retention keeps two, gate each check incl. NaN, rollback with/without previous, alias moves with an in-memory Qdrant client) and the repo checks incl. the Spark suite in a container
- [ ] 1.2 team-ai: pointer-aware `RecommendationCache` with 5 s memo and fallback (D5); verify unit tests (pointer set → gen keys and model_version, absent → legacy keys, memo expiry), `make check` + `make test`

## 2. E2E track (platform-e2e; platform-recsys and team-ai FEATURES.yaml)

- [ ] 2.1 One FEATURES.yaml acceptance line per scenario, `planned` until merged; verify `features-check`
- [ ] 2.2 `recommendations/generation_publish.feature`: second promotion keeps previous, third drops oldest, rejected candidate leaves serving, one-size-fits-all rejected, rollback restores, rollback refused, serving follows within 10 s; run the real recsys image on dataset fixtures (`recsys_job_driver.py`) and read recommendations through the gateway; mark scenarios that rewrite the shared serving pointer `@destructive` (they change what every other recommendation scenario sees); verify they pass
- [ ] 2.3 The existing recommendation scenarios stay green after the generation switch

## 3. Review and verify

- [ ] 3.1 `contract-boundary-reviewer` (recsys owns its keys/collections; team-ai reads only; no proto change); verify no blocking finding
- [ ] 3.2 Gate: `openspec validate recsys-generation-publish --strict`, `features.py --strict`, `spec_sync.py recsys-generation-publish --strict`, `repo_doctor`
- [ ] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane

## 4. Archive

- [ ] 4.1 `openspec archive recsys-generation-publish`; verify `openspec list`
