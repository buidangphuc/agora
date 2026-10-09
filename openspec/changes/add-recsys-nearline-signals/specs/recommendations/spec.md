## ADDED Requirements

### Requirement: Nearline real-time session signals

The system SHALL maintain real-time user session signals and recent interactions in Redis updated within seconds of tracking event receipt.

#### Scenario: User recent views update nearline signals

- **WHEN** a user views item `item-A` and then `item-B`
- **THEN** the nearline signal layer records `[item-B, item-A]` in the user's recent items list in Redis and increments the respective category affinities

#### Scenario: Real-time item co-occurrence is tracked

- **WHEN** multiple users view `item-A` and `item-B` within the same session
- **THEN** the co-view count between `item-A` and `item-B` is incremented in Redis

### Requirement: Serving ranks with the nearline signals

The recommendation serving path SHALL read the position-debiased CTR from the nearline Redis keys (`recs:nearline:ctr:<listing_id>`,
see design.md) for the candidates of a GBDT-ranked request, and SHALL rank with it. When the nearline store is unreachable, slow, or has
no usable data for a candidate, serving SHALL continue on the prior CTR without failing or degrading the request.

#### Scenario: Serving ranks with the nearline CTR written by the consumer

- **WHEN** two home-feed candidates have equal model scores and only the second has a nearline CTR in Redis
- **THEN** a `Recommend` call through the gateway ranks the second above the first
- **AND** with no nearline row for either, the order is the candidates' own order

#### Scenario: Nearline outage leaves the request served

- **WHEN** the nearline Redis cannot be read, or has only rows below the impression floor
- **THEN** the request is answered with the prior ordering and a non-degraded status
- **VERIFIED BY**: team-ai/tests/unit/modules/recommend/test_factory_serving_wiring.py › test_nearline_outage_does_not_fail_or_degrade_the_request, test_nearline_below_the_impression_floor_is_not_used; team-ai/tests/unit/modules/recommend/test_redis_nearline_store.py. Not verifiable end to end: it needs the stack's Redis (also the serving cache) to fail and `status` is not on the gateway wire.

