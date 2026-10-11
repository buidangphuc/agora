## Why

team-ai keeps the tag taxonomy (canonical tags, exploring candidates, synonym index) in process memory. A promotion is
an operator decision that changes the filter facets of the catalogue, and it vanishes on every restart or deploy and
differs between replicas (README known gap). The registry is also shared by the REST routes and by gRPC `ClassifyTags`,
which team-search's indexer calls, so a lost promotion silently changes indexed facets.

## What Changes

- **team-ai**: with `TAXONOMY_PERSISTENCE_ENABLED=true` the registry is persisted in team-ai's Redis, in its own database
  (`TAXONOMY_REDIS_DATABASE`, default 5; the existing users are 0 serving/chat/rate limit, 2 featurestore, 3-4 and 10-13
  e2e) under the prefix `tagtax:v1:`:
  - loaded at startup over the code seed (persisted canonical tags win, so promoted synonyms survive);
  - `promote` writes durably before it answers: if the store write fails the call answers 503 and changes nothing;
  - `explore` writes the candidates it registered (best effort: a store failure is logged and the call still answers, the
    candidates are re-discoverable);
  - REST and gRPC `ClassifyTags` keep sharing the one in-process registry, so both see the loaded and promoted state.
- A store outage at startup does not stop the service: it serves the seed taxonomy and retries the load before the next
  `promote` or `explore` applies.
- Off by default (unchanged in-memory behaviour); the local compose turns it on (design.md).
- Candidates registered as a side effect of online `classify` are not persisted (hot path stays a pure in-memory
  computation).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tag-taxonomy-enrichment`: ADDED requirement "Tag taxonomy survives a restart".

## Impact

- Code: `team-ai/app/modules/business/tag_classifier/` (`service.py`, new `store.py`, `factory.py`), `app/bootstrap/addons.py`, `app/core/config/ai.py` + `__init__.py`, `.env.example`, README.
- Compose (integrator): team-ai env `TAXONOMY_PERSISTENCE_ENABLED=true` (+ the existing `REDIS_ENABLED=true`).
- E2E: `@destructive` scenario in `ai/tag_classifier_taxonomy.feature` (restart `agora-team-ai-svc`), `tax_steps.py`, `team-ai/FEATURES.yaml`.

## Non-goals

- No cross-replica live sync (a replica loads at startup and on the next mutation; no pub/sub, no invalidation).
- No new store type (no Postgres table: team-ai runs with `DATABASE_ENABLED=false`), no migration tooling, no un-promote.
- Depends on `tag-routes-authz` only for the e2e (promotion needs the admin token); the code is independent.
