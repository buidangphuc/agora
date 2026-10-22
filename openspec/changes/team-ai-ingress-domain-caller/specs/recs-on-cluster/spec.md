## MODIFIED Requirements

### Requirement: team-ai serves recommendations from the cluster stores

team-ai's cluster values SHALL point at the in-cluster Qdrant and valkey
Services: the collection and cache contract the recsys job writes, featurestore online features on Redis DB 2, and nearline
signals on Redis DB 0. team-ai's ingress allow-list SHALL stay `team-gateway`, `team-search-indexer`, `team-domain` and `prometheus`.

#### Scenario: team-ai points at the cluster stores

- **WHEN** `charts/service` is rendered for team-ai with its service values
- **THEN** the container env has `RECS_QDRANT_URL` naming Service `qdrant`
  and `RECS_QDRANT_COLLECTION` equal to the recsys item collection, `RECS_FEATURESTORE_REDIS_URL` on `valkey` DB 2,
  `RECS_NEARLINE_REDIS_URL` on `valkey` DB 0, and the cache prefix, schema and Redis host match the recsys job config

#### Scenario: team-ai keeps its ingress allow-list

- **WHEN** `charts/service` is rendered for team-ai with its service values
- **THEN** its NetworkPolicy ingress sources are exactly `team-gateway`, `team-search-indexer`, `team-domain` and `prometheus`
