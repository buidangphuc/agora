## Why

recsys publishes generation N+1 by moving the Qdrant aliases first and the Redis serving/previous pointers second.
team-ai reads the Redis pointer for its pre-computed lists but queries Qdrant through the alias, so for a few
milliseconds on every publish (and indefinitely if the job crashes between the two steps) it can combine generation N
lists with generation N+1 vectors. The archived `recsys-generation-publish` review recorded this as a follow-up. Separately,
team-ai's local compose entry carries another service's (listing) database credentials as literals.

## What Changes

- team-ai resolves the Qdrant collection (`<RECS_QDRANT_COLLECTION>__<generation>`) from the same serving-generation
  pointer it uses for the Redis keys. The pointer is read once per request and pinned for the whole request, so lists and
  vectors always belong to one generation. The alias name is used only when no pointer exists (legacy / first boot).
- The Qdrant aliases are kept for one release as a compatibility shim for readers that predate this change (a reverted
  team-ai), exactly like `RECS_WRITE_LEGACY_KEYS`. They no longer decide anything for the current team-ai; a crash
  between the alias move and the pointer switch is harmless to it. Rollback still converges aliases, pointers and registry.
- Retention (`prune_generations`) never deletes a collection or key set named by `recs:v1:serving` or `recs:v1:previous`,
  and deletes nothing when no generation can be named (unreadable pointers).
- Local compose: team-ai's `POSTGRES_*` env stops borrowing listing's literal credentials; values come from
  `${TEAM_AI_POSTGRES_*:-placeholder}` interpolation with a documented root `.env.example`, the pattern already used for
  `SEED_ADMIN_PASSWORD`.
- No proto change. No new Redis/Qdrant naming.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `recsys-generations`: serving resolves lists and vectors from one pointer; aliases become a compatibility shim;
  retention is pointer-safe.
- `deploy-runtime`: local compose holds no literal foreign DB credentials for team-ai.

## Impact

- `team-ai/app/modules/business/recommend/{cache,backends,factory,service}.py` and unit tests.
- `platform-recsys/recsys/load/{qdrant,redis_cache}.py`, `recsys/publish.py` docstrings, README, unit tests.
- `docker-compose.services.yaml` (team-ai env only), new root `.env.example`.
- `platform-e2e`: `recommendations/serving_switch_atomicity.feature`, `ssa_steps.py`, FEATURES.yaml entries in
  platform-recsys, team-ai and the compose item (repo owning `deploy-runtime` features).
- Needs a team-ai image rebuild and a compose recreate of `team-ai-svc` (done by the integrator).
