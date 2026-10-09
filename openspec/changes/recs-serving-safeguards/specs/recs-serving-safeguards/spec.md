## ADDED Requirements

### Requirement: Recommendations degrade instead of failing

When reading the cache or Qdrant fails, `Recommend` SHALL still answer `OK`. It SHALL return the serving generation's
popular list, or an empty list if that cannot be read either, with `model_version` `serving-fallback`. It SHALL answer
`UNAVAILABLE` only when recommendations are disabled (`RECS_ENABLED=false`).

#### Scenario: Recommendations survive a Redis outage

- **WHEN** the Redis used by team-ai is stopped and a buyer requests homepage recommendations through the gateway
- **THEN** the call succeeds with `model_version` "serving-fallback"

### Requirement: Cold start serves the producer's popular list

A user with no precomputed recommendations SHALL be served the serving generation's popular list, in the producer's
order. If the serving pointer is absent, the unscoped popular list SHALL be used. The list SHALL NOT be an arbitrary
sample of the vector collection.

#### Scenario: A new buyer gets the popular list

- **WHEN** the serving generation's popular list is L and a buyer with no recommendations of their own requests homepage
  recommendations
- **THEN** the returned listing ids are the first items of L, in L's order

### Requirement: Serving refuses an in-process catalogue outside local

team-ai SHALL refuse to start with `RECS_ENABLED=true` and `RECS_BACKEND=memory` when `ENVIRONMENT` is anything other
than dev, local or test. The error SHALL name `RECS_BACKEND`.

#### Scenario: Production refuses the memory recommendation backend

- **WHEN** the team-ai image is started with `ENVIRONMENT=production`, `RECS_ENABLED=true` and `RECS_BACKEND=memory`
- **THEN** the process exits non-zero and its log names `RECS_BACKEND`

### Requirement: Every gRPC entrypoint serves recommendations

Every way of starting team-ai's gRPC server SHALL register the recommendation service with the same configuration.

#### Scenario: The standalone gRPC server answers Recommend

- **WHEN** team-ai is started with `scripts/run_grpc.py` and a buyer requests recommendations through a gateway pointed at
  it
- **THEN** the call succeeds instead of answering `unimplemented`

### Requirement: A recommendation response identifies itself for attribution

Every `RecommendResponse` SHALL carry the placement the server served (`placement_id`) and a new `request_id` minted for
that response. team-ai SHALL log one `recs.served` line per response with the request id, placement, model version, the
returned listing ids and whether it was a fallback. The storefront SHALL send the response's `request_id` as the
`impressionId` and its `placement_id` as the `placementId` on the impression and click beacons of that recommendation
row.

#### Scenario: Two calls get distinct request ids and the served placement

- **WHEN** a buyer requests homepage recommendations twice through the gateway
- **THEN** both responses have `placement_id` "home_feed" and two different non-empty `request_id`s, and team-ai logged a
  `recs.served` line for each

#### Scenario: Storefront beacons carry the server request id

- **WHEN** a buyer opens the homepage and its recommendation row is shown
- **THEN** the impression beacons of that row carry an `impressionId` equal to the `request_id` the storefront received,
  and `placementId` "home_feed"

### Requirement: Ranking uses online features when present

When `RECS_FEATURESTORE_REDIS_URL` is set, ranking SHALL read each candidate's `item_popularity` features of the version
named by `fs:item_popularity:current`. Among candidates with equal model scores, a higher `ctr_7d` and then a higher
`favorites_current` SHALL rank first. Missing keys or a feature store error SHALL leave ranking as without features.

#### Scenario: Online features break a tie

- **WHEN** two candidates have equal model scores and only the second has `item_popularity` features with `ctr_7d` 0.5
- **THEN** the second ranks above the first in the response
