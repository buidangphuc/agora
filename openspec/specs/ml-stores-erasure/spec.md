# ml-stores-erasure Specification

## Purpose
TBD - created by archiving change analytics-user-erasure. Update Purpose after archive.

## Requirements

### Requirement: platform-recsys erases the user's serving state

A long-running `python -m recsys.erasure` process SHALL consume `identity.events` in group `platform-recsys.erasure`, act only on `platform.identity.v1.UserErased`, and delete the Redis keys `recs:v1:gen:*:user:<id>`, `recs:v1:user:<id>`, `recs:nearline:user:<id>:items` and `recs:nearline:user:<id>:cats`, and every Qdrant point with payload `user_key` = id in each collection named `user_als_vectors__*`. Offsets SHALL be committed only after the deletions succeeded. Undecodable records and a `UserErased` with an empty `user_id` SHALL go to `identity.events.recsys.dlq` and be committed past. Other envelope types SHALL be skipped.

#### Scenario: AUE-19 Recommendation and nearline keys are removed
- **WHEN** a `UserErased` for U arrives while Redis holds U's keys under two generations, the legacy key and both nearline keys, and Qdrant holds U's vector in the serving user collection
- **THEN** within 60 s zero Redis keys and zero Qdrant points contain U's id, and V's keys and points are unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-19 Recommendation and nearline keys are removed`

#### Scenario: AUE-20 Redis or Qdrant failure exits without commit
- **WHEN** Redis or Qdrant is unreachable while handling the event
- **THEN** the process exits non-zero with the offset uncommitted and the restart replays the event to completion
- **VERIFIED BY**: `platform-recsys/tests/test_erasure.py` `test_aue20_redis_failure_exits_without_commit_and_restart_replays`

#### Scenario: AUE-21 Redelivery and missing keys are no-ops
- **WHEN** the same event is delivered twice, or arrives for a user with no keys
- **THEN** both deliveries succeed, delete nothing further and commit the offset, with no DLQ record
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-21 Redelivery and missing keys are no-ops`

#### Scenario: AUE-22 Poison record goes to the DLQ
- **WHEN** an undecodable record and a `UserErased` with an empty `user_id` are consumed
- **THEN** both are published to `identity.events.recsys.dlq` and committed past, and no key is deleted
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-22 Poison record goes to the DLQ`

#### Scenario: AUE-23 Logs carry no raw id
- **WHEN** the event for U is handled with debug logging
- **THEN** the log output contains zero occurrences of U's id and reports deleted-key counts
- **VERIFIED BY**: `platform-recsys/tests/test_erasure.py` `test_aue23_logs_carry_no_raw_id_and_report_counts`

### Requirement: platform-featurestore drops pseudonymised entities and stale user rows

The featurestore input views `events`, `facts` and `orders` SHALL exclude rows whose `user_key`, `user_id` or `buyer_id` starts with `erased:`; `registry/sql/*` and `registry/features.lock` SHALL stay unchanged. After writing the new rows and before moving the `current` and `meta` pointers, `materialize` SHALL delete every online key of the views `user_activity` and `user_preferences` whose entity is absent from the new snapshot. The offline snapshot and the `als_interactions@v1` dataset SHALL contain no `erased:` entity.

#### Scenario: AUE-24 A user erased upstream disappears from the online store
- **WHEN** `materialize` runs on inputs where U's rows were re-keyed to `erased:<hex>` and Redis DB 2 still holds `fs:user_activity:v2:<U>` and `fs:user_preferences:v1:<U>` from the previous run
- **THEN** both keys are gone, no key with entity `erased:<hex>` exists, other users' keys are present, and the parity gate passes
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-24 A user erased upstream disappears from the online store`

#### Scenario: AUE-25 Pseudonymised rows do not become features or training rows
- **WHEN** `materialize` and `dataset` run on inputs containing rows with `user_key` = `erased:<hex>`
- **THEN** the offline `user_activity`, `user_preferences` and `als_interactions` outputs contain zero rows for that key and `paid_orders_30d` counts nobody for `buyer_id` = `erased:<hex>`
- **VERIFIED BY**: `platform-featurestore/tests/test_erasure_inputs.py` `test_pseudonymised_rows_excluded_from_views_and_dataset_AUE_25`

#### Scenario: AUE-26 Item views and registry hashes are unchanged
- **WHEN** `materialize` runs and `registry/features.lock` is verified
- **THEN** the item views' online keys are not deleted by the reconcile and the lock check passes without a hash change
- **VERIFIED BY**: `platform-featurestore/tests/test_erasure_inputs.py` `test_item_views_and_registry_hashes_unchanged_AUE_26`

#### Scenario: AUE-27 A Redis failure during the reconcile fails the run
- **WHEN** Redis errors while deleting stale user keys
- **THEN** `materialize` exits non-zero and does not write the `current` and `meta` pointers
- **VERIFIED BY**: `platform-featurestore/tests/test_erasure_reconcile.py` `test_redis_failure_during_reconcile_fails_run_AUE_27`

### Requirement: team-ai erases the user's chat and counter state

team-ai SHALL run an erasure addon on its aiokafka stack consuming `identity.events` in group `team-ai.user-erasure`, acting only on `platform.identity.v1.UserErased`, and SHALL delete `chat:session:<quote(id)>:*` and every key under `QUOTA_REDIS_PREFIX` and `RATE_LIMIT_REDIS_PREFIX:principal` that embeds the principal `user:<id>`. Offsets SHALL be committed after the deletions. Undecodable records and an empty `user_id` SHALL go to `identity.events.team-ai.dlq`. A different user's keys, including one whose id has the erased id as a prefix, SHALL NOT be deleted.

#### Scenario: AUE-28 Chat sessions, quota and rate-limit keys are removed
- **WHEN** a `UserErased` for U arrives while Redis holds three chat sessions, a quota counter and a rate-limit counter of U
- **THEN** within 60 s none of those keys exists and the same keys of user V are present
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-28 Chat sessions, quota and rate-limit keys are removed`

#### Scenario: AUE-29 A prefix-sharing id is not matched
- **WHEN** U's id is `abc` and another user has id `abcd` with a chat session
- **THEN** the session of `abcd` is present after U's erasure
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-29 A prefix-sharing id is not matched`

#### Scenario: AUE-30 Redis failure retries without commit
- **WHEN** Redis is unreachable during the deletion
- **THEN** the offset is not committed and the event is retried by the supervised consumer until it succeeds
- **VERIFIED BY**: `team-ai/tests/unit/modules/erasure/test_erasure_consumer.py` `test_aue_30_redis_failure_retries_without_commit`

#### Scenario: AUE-31 Poison record goes to the DLQ
- **WHEN** an undecodable record and a `UserErased` with an empty `user_id` are consumed
- **THEN** both are published to `identity.events.team-ai.dlq`, no key is deleted and the offsets advance
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-31 Poison record goes to the DLQ`

#### Scenario: AUE-32 No raw id in logs
- **WHEN** U's erasure is processed with debug logging
- **THEN** the log output contains zero occurrences of U's id
- **VERIFIED BY**: `team-ai/tests/unit/modules/erasure/test_erasure_consumer.py` `test_aue_32_erasure_logs_carry_no_raw_id`

### Requirement: Local stack wires the erasure consumers

`docker-compose.services.yaml` SHALL create topics `identity.events.analytics.dlq`, `identity.events.recsys.dlq` and `identity.events.team-ai.dlq`, pass the consumer environment to each service, and start the recsys erasure process as a long-running non-root service. With every service up, a `UserErased` on `identity.events` SHALL leave all three DLQs empty.

#### Scenario: AUE-33 End to end across the stores
- **WHEN** a `UserErased` for a seeded user is produced, then one Parquet export cycle and one `materialize` run complete
- **THEN** the warehouse, Parquet files, Redis (recs, nearline, featurestore, chat session) and Qdrant contain zero occurrences of the user id and the three DLQ topics have zero records
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-33 End to end across the stores`

#### Scenario: AUE-34 A missing DLQ topic stops the consumer
- **WHEN** a consumer starts and its DLQ topic is absent
- **THEN** the topic-init job has created it before the consumer service becomes healthy, otherwise the service exits non-zero rather than running without a DLQ
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature` `AUE-34 A missing DLQ topic stops the consumer`
