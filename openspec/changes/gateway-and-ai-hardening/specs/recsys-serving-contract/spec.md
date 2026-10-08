## Purpose

Pins the data contract between the offline producer (`platform-recsys`) and the online consumer (`team-ai`
`Recommend`): how a listing id maps to a Qdrant point, which payload fields exist, where the model version and the
popularity list come from, and what the consumer must not claim. The contract is verified against the producer's
real point format so the two sides cannot drift silently.

## ADDED Requirements

### Requirement: Qdrant points are addressed by a deterministic id derived from the listing id

The producer SHALL store each item vector under point id `uuid5(NAMESPACE, listing_id)` where `NAMESPACE` is the
fixed UUID `6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00`, and the consumer SHALL derive the same point id from a seed
listing id before any Qdrant lookup by id. The listing id `listing-1` SHALL map to
`25a4b2d5-6531-5357-90f2-06e92d1e1191` on both sides, pinned by a test in each repository.

#### Scenario: Seed lookup finds the producer's point

- **WHEN** the producer has loaded a point for `listing-1` and the consumer is asked for items similar to seed
  `listing-1`
- **THEN** the consumer addresses point `25a4b2d5-6531-5357-90f2-06e92d1e1191` and receives neighbours from the
  producer's collection (not an empty result)

#### Scenario: Both repositories pin the same id

- **WHEN** the producer's test and the consumer's test each compute the point id for `listing-1`
- **THEN** both equal `25a4b2d5-6531-5357-90f2-06e92d1e1191`, and changing the namespace in either repository
  fails its own test

### Requirement: The consumer returns the listing id from the payload

Every candidate the consumer builds from a Qdrant hit SHALL take its `listing_id` from the point payload field
`listing_id`, never from the point id. A hit without a payload `listing_id` SHALL be skipped and counted, not
returned. The seed listing SHALL be excluded from the result by its listing id.

#### Scenario: Hits are reported as real listing ids

- **WHEN** a similar-items query returns producer-format points
- **THEN** each returned `listing_id` equals the producer's source listing id and no UUID appears in the response

#### Scenario: A malformed point is skipped

- **WHEN** a hit has no `listing_id` in its payload
- **THEN** it is omitted from the result and the remaining hits are still returned

### Requirement: The model version comes from the producer

`RecommendResponse.model_version` SHALL be, in order of preference: the value of the Redis key
`{prefix}:{schema}:model_version` written by the producer (read through a short in-process cache so the request
path adds no extra round trip), else the `model_version` payload of a Qdrant hit that contributed to the result,
else the configured fallback label `RECS_MODEL_VERSION`. A new producer generation SHALL be reflected in responses
within the cache interval without a restart.

#### Scenario: Version follows the producer

- **WHEN** the producer writes `recs:v1:model_version = als-20260930T020000Z`
- **THEN** a `Recommend` response served from the cache, from ANN or from popularity carries that version

#### Scenario: Missing key falls back without failing

- **WHEN** the model version key is absent or Redis is unreachable
- **THEN** the response carries the ANN hit's payload version when there is one, otherwise `RECS_MODEL_VERSION`,
  and the RPC still succeeds

#### Scenario: A new generation is picked up

- **WHEN** the producer overwrites the model version key while `team-ai` keeps running
- **THEN** responses carry the new version after the cache interval elapses

### Requirement: Popularity comes only from the producer's popularity list

The popularity stage SHALL read only `{prefix}:{schema}:popular` from Redis. If that key is missing, expired or
malformed the stage SHALL return an empty list (logged), and the consumer SHALL NOT substitute an arbitrary set of
Qdrant points as "popular".

#### Scenario: Popular list is served in producer order

- **WHEN** `recs:v1:popular` holds `[{listing_id: "a", score: 9}, {listing_id: "b", score: 5}]` and a caller has no
  user id and no seed
- **THEN** the response lists `a` then `b` with ranks 1 and 2

#### Scenario: No popularity data yields an empty result, not arbitrary items

- **WHEN** `recs:v1:popular` is absent and the caller has no user id and no seed
- **THEN** the response contains no items and no Qdrant scroll is issued

### Requirement: Item availability is not claimed by the recommender

`team-ai` SHALL apply an availability filter only when a cache entry or point payload explicitly carries
`in_stock=false`. It SHALL NOT report stock state it does not have. Authoritative stock and existence filtering is
the responsibility of the caller that hydrates listing cards (Rule 3), and the serving contract SHALL state that
the producer does not write stock.

#### Scenario: Explicitly unavailable candidates are dropped

- **WHEN** a candidate carries `in_stock=false`
- **THEN** it is removed from the result

#### Scenario: Absent stock field is treated as unknown, not as a promise

- **WHEN** the producer's candidates carry no `in_stock` field
- **THEN** they are returned, and the response makes no stock claim that the hydrating caller must trust

### Requirement: The contract is tested with the producer's real point format

`team-ai` SHALL have tests that load points in the producer's exact format (UUID5 id; payload
`listing_id`, `model_version`, `updated_at`; Cosine vectors) into a real Qdrant client implementation and exercise
the consumer's retrieval path, plus Redis fixtures using the producer's key names and JSON shape.
`platform-recsys` SHALL have a test pinning the point id, payload keys and Redis key names that the consumer relies
on.

#### Scenario: Consumer path works on producer-format data

- **WHEN** the test loads three producer-format points and queries similar items for the first listing
- **THEN** the other two listing ids are returned ranked by similarity, the seed is excluded, and each result is a
  plain listing id

#### Scenario: Producer test guards the shared keys

- **WHEN** the producer's payload key names or Redis key names change
- **THEN** the producer's contract test fails until the consumer contract is updated deliberately

### Requirement: Every serving entrypoint registers Recommend

Each supported way to run the team-ai gRPC server (`make grpc` and the HTTP app with `GRPC_ENABLED=true`) SHALL
register `RecommendationService` with the same provider, so that with `RECS_ENABLED=true` `Recommend` does not
answer `UNAVAILABLE` on one entrypoint and succeed on another.

#### Scenario: make grpc serves recommendations

- **WHEN** the server is started through the `make grpc` entrypoint with `RECS_ENABLED=true` and populated stores
- **THEN** `Recommend` returns recommendations, and with `RECS_ENABLED=false` it returns `UNAVAILABLE` naming the
  flag

#### Scenario: Both entrypoints agree

- **WHEN** the same request is sent to a server from each entrypoint over the same stores
- **THEN** the responses are identical
