## ADDED Requirements

### Requirement: Tag taxonomy survives a restart

When taxonomy persistence is enabled, team-ai SHALL store its canonical tags and exploring candidates in Redis and load them at startup, so that a tag promoted through `POST /api/v1/ai/tags/promote` is still canonical, and an explored candidate still a candidate, after team-ai restarts. A promotion SHALL be durable before it is acknowledged: if the store cannot be written the call SHALL answer 503 and leave the taxonomy unchanged. The REST routes and gRPC `ClassifyTags` SHALL answer from the same registry. A store outage at startup SHALL NOT stop the service, which serves the seed taxonomy until the store can be read. With persistence disabled the registry stays in memory.

#### Scenario: Promoted and exploring tags survive a restart

- **WHEN** an admin promotes a tag, a second candidate is explored but left exploring, and team-ai is restarted
- **THEN** after the restart the promoted tag is canonical (listed as promoted and carried by a classification, with its bound synonym) and the other is still an exploring candidate

#### Scenario: A failed store write fails the promotion

- **WHEN** the taxonomy store cannot be written during `promote`
- **THEN** the call answers 503 and the tag is still only a candidate
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_promote_is_refused_and_unchanged_when_the_store_write_fails. Not verifiable end to end: failing Redis would break every service sharing the stack (design.md).

#### Scenario: A store outage at startup serves the seed taxonomy

- **WHEN** the store cannot be read at startup and later recovers
- **THEN** the service starts with the seed taxonomy, and the next `promote` or `explore` loads the stored state before applying its change
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_startup_outage_serves_seed_then_loads_before_the_next_mutation. Not verifiable end to end: same reason (design.md).

#### Scenario: REST and gRPC share one registry

- **WHEN** a tag is promoted through the REST route
- **THEN** gRPC `ClassifyTags` returns it for a matching title, and a restart restores it for both
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_rest_promotion_is_seen_by_grpc_classify_and_survives_a_new_process. Not verifiable end to end: the gateway does not route `ClassifyTags` (internal, service principal only).

#### Scenario: Persistence is off unless enabled

- **WHEN** `TAXONOMY_PERSISTENCE_ENABLED` is false
- **THEN** nothing is written to Redis and a restart returns to the seed taxonomy, and enabling it without `REDIS_ENABLED` or on the same Redis database as `REDIS_DATABASE` fails startup
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_disabled_persistence_writes_nothing, test_enabling_persistence_requires_redis_on_another_database. Not verifiable end to end: the stack runs with persistence on (design.md).
