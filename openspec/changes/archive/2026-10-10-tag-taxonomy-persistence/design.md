## Context

See proposal.md. `TagClassifierService` holds `_canonical_tags`, `_candidate_tags` and `_synonym_index`; one process-wide
instance (`shared_tag_classifier()`) serves both the REST routes and the gRPC servicer. team-ai already has Redis
(`REDIS_ENABLED`, compose `redis:6379`) and `DATABASE_ENABLED=false`, so Redis is the only durable store it owns.

## Decisions

- **D1: Redis, own database and prefix.** `TAXONOMY_REDIS_DATABASE` (default 5) via a client built from the same host
  settings; startup refuses the same database as `REDIS_DATABASE` (rate limit, chat history). Alternative rejected:
  `recs:*` keys in DB 0 (owned by platform-recsys, different lifecycle), Postgres (team-ai has none enabled).
- **D2: per-tag hashes, not one blob.** `tagtax:v1:canonical` and `tagtax:v1:candidates` are hashes `slug -> TagItem JSON`.
  A write touches only the tags it changed (`HSET`, `HDEL` of the promoted candidates in one `MULTI`), so two replicas
  promoting different tags do not overwrite each other. The version segment lets a future format coexist.
- **D3: overlay over the seed.** The seed stays in code; persisted canonical tags are registered after it and win (a
  seed tag with promoted synonyms comes back with them). Only changed tags are persisted, so a new seed tag shipped in a
  release appears without a migration.
- **D4: promote is persist-first.** The new state is computed on copies, written, and only then applied in memory; a
  store failure raises `ServiceUnavailableError` (503) with the registry unchanged. A lock serializes `promote`/`explore`
  (they now await). `explore` is best effort (candidates are re-discoverable and counts only grow).
- **D5: startup load fails open, mutation does not.** Reads work from the seed during an outage; the next mutation retries
  the load first so it never writes over state it has not seen.
- **D6: wiring.** A `TagTaxonomyAddon` (enabled by `TAXONOMY_PERSISTENCE_ENABLED`, requires `REDIS_ENABLED`) attaches the
  store to `shared_tag_classifier()` at startup and closes its client at shutdown; REST and gRPC both read that instance.
  The standalone `make grpc` entry has no addons and stays in-memory.

## Why three scenarios are not end-to-end tests

Failing or stopping the shared Redis would break every service on the stack, the gateway does not route `ClassifyTags`,
and the stack always runs with persistence on. They carry `VERIFIED BY` lines and `not-testable` FEATURES entries.

## Deployment needs (team-ai)

| Variable | Value | Why |
|---|---|---|
| `TAXONOMY_PERSISTENCE_ENABLED` | `true` | turns the store on |
| `REDIS_ENABLED`, `REDIS_HOST` | `true`, `redis` | already set in the root compose |
| `TAXONOMY_REDIS_DATABASE` | `5` (default) | unused database |

The `@destructive` e2e scenario restarts the container `agora-team-ai-svc` (`AI_CONTAINER` overrides) and waits for
`/readyz`; it also needs the admin token of `tag-routes-authz`.

## Risks / Trade-offs

- A replica that started before a promotion serves the old taxonomy until its own restart or next mutation → accepted
  (non-goal: live sync); the facets are only read by the indexer at index time.
- Online `classify` candidates are in memory only → they re-appear on the next explore.
