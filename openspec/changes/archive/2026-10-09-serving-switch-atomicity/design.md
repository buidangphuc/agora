## Context

`publish_generation`: write Redis gen keys, write Qdrant gen collections, `activate_aliases`, then one Lua EVAL moves
`serving`/`previous`/`model_version`. The two switches are two systems and cannot be one transaction. team-ai's
`PrecomputedCache` already memoises the serving pointer for 5 s; `QdrantRetrievalBackend` queries the fixed alias name.

## Decisions

**D1. One pointer decides both stores.** The Redis pointer is the only switch that matters, because it is the only
atomic one. team-ai derives the collection name from it: `f"{RECS_QDRANT_COLLECTION}__{generation}"`. The backend gets a
`generation_source` (the cache's `serving_generation`) so there is one memo and one read. When the pointer is absent or
Redis errors (treated as absent, as the cache already does), the collection is the alias name `RECS_QDRANT_COLLECTION`
(legacy / pre-first-publish). A pointer naming a generation whose collection is missing is NOT papered over with the
alias (that would reintroduce the mix): retrieval fails open to an empty list, the existing ladder degrades, and the
startup/re-check `collection_ok` reports the named collection.

**D2. Pinned per request.** The 5 s memo can expire between the user-list read and the vector query of one request.
`RecommendationService.recommend` pins the generation (a `ContextVar` set by `PrecomputedCache.pin_generation()`, reset in
`finally`) so every cache and backend read of that request uses the same value. Backends/caches without the hook (test
fakes, memory backend) are unaffected.

**D3. Aliases stay, as a shim, for one release.** Alternatives: remove them now (breaks a reverted team-ai and any
operator query by alias; rollback of the team-ai image is the safety net we want) or keep them authoritative (the bug).
Kept, still moved before the pointer, still converged by rollback `_converge`. Documented as deprecated; removal is a
follow-up once the pre-change team-ai image is out of the rollback window. The legacy-collection migration is unchanged.

**D4. Retention is pointer-safe.** `prune_generations(keep)` already spares `keep` and alias targets. Now it also
refuses to run with an empty `keep` (no serving/previous readable means "do not know", not "delete everything"), so the
safety no longer depends on aliases existing. `publish_generation` computes `keep` after the switch (unchanged) and
skips pruning when nothing is named. A collection named by serving or previous is never deleted, even if no alias points
at it.

**D5. Rollback.** Unchanged ordering (aliases, CAS swap, registry); since team-ai follows the pointer, the swap alone
changes what is served within 5 s. A crash after the alias move leaves the pointer old: team-ai still serves one
consistent (old) generation, and rerunning converges.

**D6. Credentials.** `POSTGRES_*` for team-ai are required settings but `DATABASE_ENABLED=false`. They become
`${TEAM_AI_POSTGRES_USER:-team_ai_svc}`, `${TEAM_AI_POSTGRES_PASSWORD:-team-ai-local-placeholder}`,
`${TEAM_AI_POSTGRES_DB:-team_ai_db}`; the root `.env.example` documents them; `.env` is already git-ignored. team-ai no
longer holds listing's DB credentials (AGENTS.md rule 3).

## Risks

- A pointer-named collection missing (manual deletion) => recommendations degrade to popularity; retention protection
  and rollback's `generation_present` check make this an operator action only.
- Mixed-version rollout: new team-ai against recsys that has not yet published a generation-named collection => pointer
  absent => alias, same as today.
