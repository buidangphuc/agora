## Purpose

Defines the online read contract of the feature store: the internal `platform.analytics.v1.FeatureService`
that serving code calls for the current values of declared features, who may call it, how fast it answers and
how it reports anything it cannot answer. Missing data is always explicit, never a silent zero.

## ADDED Requirements

### Requirement: GetOnlineFeatures returns declared features for a batch of entities

The system SHALL answer `GetOnlineFeatures` for one entity type, a list of up to 500 entity ids and a list of
up to 50 feature references, with one result per (entity, reference) carrying a status and, only when the
status is `OK`, a typed value and its effective as-of time (the later of the time the value was written and
the feature's last successful materialization run, which re-evaluates every entity whose window changed). The
statuses SHALL be `OK`, `MISSING` (no value for that entity), `STALE` (effective as-of time older than the
feature's TTL; value withheld), `PENDING` and `RETIRED`. A reference
that is not registered, or whose entity differs from the request's entity type, SHALL fail the whole call with
`INVALID_ARGUMENT` naming it. Requests above the limits SHALL fail with `INVALID_ARGUMENT`.

#### Scenario: Values for several items in one call

- **WHEN** an authorized service requests `item.views_7d@v1` and `item.trending_1h@v1` for items L1 and L2,
  both of which have fresh online values
- **THEN** the response has four results, each `OK` with a value and an as-of time

#### Scenario: An entity with no value is reported missing, not zero

- **WHEN** an authorized service requests `item.views_7d@v1` for a listing that has never had an event
- **THEN** that result has status `MISSING` and carries no value

#### Scenario: A value older than its TTL is reported stale

- **WHEN** the effective as-of time of `item.trending_1h@v1` for L1 is older than that feature's TTL (the
  online materializer has not completed a run for longer than the TTL)
- **THEN** that result has status `STALE`, carries the effective as-of time and no value

#### Scenario: An unknown or mistyped reference fails loudly

- **WHEN** a request names `item.views_7d@v9` or asks for `user.event_count_30d@v1` with entity type `item`
- **THEN** the call fails with `INVALID_ARGUMENT` naming the reference

#### Scenario: The online store is unreachable

- **WHEN** valkey/Redis is unreachable during a call
- **THEN** the call fails with `UNAVAILABLE` rather than returning results marked missing

### Requirement: Only service principals with features.read may read features

The system SHALL require the forwarded principal to be of type SERVICE and to hold the scope `features.read`
for `GetOnlineFeatures` and `DescribeFeatures`, and the scope `features.dataset` for `BuildDataset` and
`GetDatasetBuild`. Both SHALL be service-only scopes granted to no user role. The gateway SHALL NOT route
`FeatureService`.

#### Scenario: A service principal with the scope is allowed

- **WHEN** `team-ai` calls `GetOnlineFeatures` as `service-team-ai` with scope `features.read`
- **THEN** the call is served

#### Scenario: A reader cannot request dataset builds

- **WHEN** `team-ai` calls `BuildDataset` holding `features.read` but not `features.dataset`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: A user principal is refused even as admin

- **WHEN** a call carries a USER principal holding `admin` but not `features.read`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: A call without a principal is refused

- **WHEN** a call carries no forwarded principal
- **THEN** the call fails with `UNAUTHENTICATED`

#### Scenario: The gateway does not expose the feature service

- **WHEN** any client, signed in or not, posts to the gateway path of `FeatureService/GetOnlineFeatures`
- **THEN** the gateway answers HTTP 501 with Connect code `unimplemented` and never reaches `team-analytics`

#### Scenario: No user role is granted the feature scopes

- **WHEN** the role table of `team-identity` is checked by its service-only scope test
- **THEN** no role grants `features.read` or `features.dataset`, and the test fails if one ever does

### Requirement: Online reads meet a latency budget and degrade explicitly on the client

The system SHALL answer `GetOnlineFeatures` for 100 entities and 20 references within 20 ms at p99 measured at
the server. The `team-ai` client SHALL call it with a deadline (default 30 ms), SHALL cache results in process
for at most the smaller of 30 s and half of the feature's freshness SLO, and on deadline or error SHALL mark
every requested value as unavailable with reason `feature_failure`, never substituting zero or a default.

#### Scenario: The server stays within its budget

- **WHEN** a load test sends `GetOnlineFeatures` for 100 entities and 20 references at the target rate
- **THEN** the server-side p99 latency is at most 20 ms

#### Scenario: The client reports feature_failure on timeout

- **WHEN** the feature service does not answer within the client deadline
- **THEN** the `team-ai` client returns every requested value as unavailable with reason
  `feature_failure`, and a metric of feature failures increases

#### Scenario: The client serves repeated reads from its cache

- **WHEN** the same entity and reference are requested twice within the cache lifetime
- **THEN** the second read does not call the feature service and returns the cached value with its
  original as-of time
