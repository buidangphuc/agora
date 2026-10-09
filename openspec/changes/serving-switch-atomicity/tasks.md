## 1. Code track

- [ ] 1.1 team-ai: collection resolved from the serving pointer, pinned per request, alias fallback when no pointer (D1, D2); verify unit tests fail without the change (collection name from pointer, alias fallback, pin across memo expiry, missing generation collection fails open) and `make check` + `make test`
- [ ] 1.2 platform-recsys: pointer-safe retention (empty keep deletes nothing; serving/previous spared without an alias), alias shim documented, rollback converge unchanged (D3-D5); verify `make test-host`
- [ ] 1.3 Compose + env: team-ai `POSTGRES_*` via `TEAM_AI_POSTGRES_*` defaults and root `.env.example` (D6); verify `docker compose config` renders and no `listing_*` literal remains in the team-ai block

## 2. E2E track

- [ ] 2.1 FEATURES.yaml acceptance line per scenario (status `planned`, note `needs rebuild`); verify `features-check`
- [ ] 2.2 `recommendations/serving_switch_atomicity.feature` + `ssa_steps.py`, serving-pointer scenarios `@destructive`; verify lint and (integrator) run after rebuild

## 3. Gate

- [ ] 3.1 `openspec validate serving-switch-atomicity --strict`, `spec_sync`, `repo_doctor`
