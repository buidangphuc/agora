## ADDED Requirements

### Requirement: A placement can name an experiment flag and assignment is stable

A placement in `placements.yaml` MAY declare `experiment.flag`. For such a placement, `Recommend` SHALL evaluate that Flipt
variant flag over Flipt's REST evaluate API with the entity id `sha256(salt + ":" + id)`, where `id` is the bound user id,
else the anonymous id. Evaluation SHALL be deterministic: the same id and the same flag state SHALL give the same variant on
every call. When the caller has neither id, or the flag is disabled or does not match, the arm SHALL be `none`.

#### Scenario: A visitor always lands in the same arm

- **WHEN** an enabled 50/50 variant flag `recs-home-feed-ranker` exists and one anonymous visitor requests homepage recommendations 5 times through the gateway
- **THEN** all 5 responses carry the same arm segment of `model_version`, and it is `recs-home-feed-ranker:control` or `recs-home-feed-ranker:similarity`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `A visitor always lands in the same arm`

#### Scenario: Both variants receive traffic

- **WHEN** 40 distinct anonymous visitors request homepage recommendations while the 50/50 flag is enabled
- **THEN** the responses contain both arm `recs-home-feed-ranker:control` and arm `recs-home-feed-ranker:similarity`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `Both variants receive traffic`

#### Scenario: A disabled flag means no arm

- **WHEN** the flag `recs-home-feed-ranker` is disabled and a buyer requests homepage recommendations through the gateway
- **THEN** the arm segment of `model_version` is `none` and the response is otherwise served as before
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `A disabled flag means no arm`

#### Scenario: A placement without a flag makes no Flipt call

- **WHEN** a buyer requests the `similar_items` placement, which declares no experiment
- **THEN** the arm segment is `none` and team-ai performs zero Flipt requests
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_placement_without_flag_makes_no_call`

#### Scenario: A caller with no identity gets no arm

- **WHEN** `Recommend` runs for a placement with a flag, with neither a user id nor an anonymous id
- **THEN** the arm segment is `none` and no Flipt request is made
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_no_identity_no_arm`

### Requirement: Flipt never sees a raw id and failure never fails the request

team-ai SHALL send Flipt only the salted SHA-256 entity id, and SHALL NOT log the raw id, the salt or the entity hash. When
Flipt is unreachable, answers with a server error, or does not answer within `RECS_FLIPT_TIMEOUT_MS` (default 50), `Recommend`
SHALL serve the placement's default ranker and stamp the arm `<flag>:unassigned`. The response status SHALL be unchanged.

#### Scenario: Flipt unreachable serves the default ranker as unassigned

- **WHEN** Flipt is stopped and a buyer requests homepage recommendations through the gateway
- **THEN** the call succeeds, the ranker segment is the placement default (`fixed_weights` or `gbdt`), and the arm segment is `recs-home-feed-ranker:unassigned`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `Flipt unreachable serves the default ranker as unassigned`

#### Scenario: A slow Flipt is cut off at the timeout

- **WHEN** Flipt answers after 200 ms and `RECS_FLIPT_TIMEOUT_MS` is 50
- **THEN** `Recommend` returns within 150 ms of the call start with arm `<flag>:unassigned`
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_slow_flipt_times_out`

#### Scenario: Flipt receives only the hashed entity id

- **WHEN** a signed-in buyer with user id `u1` requests recommendations for a flagged placement
- **THEN** the evaluate request body `entityId` equals `sha256(salt + ":u1")` and neither `u1` nor the salt appears in the request or in any team-ai log line of the call
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_entity_id_is_salted_hash_and_not_logged`

#### Scenario: A server error from Flipt is treated as unassigned

- **WHEN** Flipt answers HTTP 500 to the evaluate call
- **THEN** the response status is OK and the arm segment is `<flag>:unassigned`
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_flipt_500_is_unassigned`

### Requirement: An arm may change the ranker only

A variant attachment MAY carry `ranking.model`; when it names an implemented ranking model, that model SHALL replace the
placement's for that request. Every other attachment key SHALL be ignored, so an arm cannot change the candidate ladder,
the generation, filters or feature-store use. An unimplemented model name SHALL be ignored and the default ranker used.

#### Scenario: The similarity arm changes the ranker segment

- **WHEN** a visitor is assigned the `similarity` variant whose attachment is `{"ranking":{"model":"cosine_rank"}}`
- **THEN** the ranker segment of `model_version` is `similarity` and the tier segment is the one the default ladder would produce
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `The similarity arm changes the ranker segment`

#### Scenario: An attachment cannot change the ladder

- **WHEN** a variant attachment is `{"ranking":{"model":"cosine_rank"},"candidate_ladder":[{"strategy":"global_popular"}],"in_stock_only":false}`
- **THEN** the candidate ladder and filters used are the placement's, and only the ranker changes
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_attachment_cannot_change_ladder`

#### Scenario: An unknown ranking model in an attachment is ignored

- **WHEN** a variant attachment is `{"ranking":{"model":"lambdamart"}}`
- **THEN** the default ranker serves and the arm segment still names the variant
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_unknown_ranking_model_ignored`

### Requirement: The experiment flags exist, switched off

The repository SHALL provide an idempotent script that creates the Flipt variant flag `recs-home-feed-ranker` with variants
`control` and `similarity` (the latter carrying the ranker attachment), disabled. Running it SHALL NOT change a flag that
exists and SHALL NOT enable any flag.

#### Scenario: Seeding creates a disabled flag and is idempotent

- **WHEN** the seed script runs twice against a Flipt without the flag
- **THEN** the flag exists with `enabled=false` and both variants, and the second run exits 0 without changing it
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `Seeding creates a disabled flag and is idempotent`

#### Scenario: Seeding does not overwrite an enabled flag

- **WHEN** the flag already exists and is enabled, and the seed script runs
- **THEN** the flag is still enabled with its rollout unchanged
- **VERIFIED BY**: team-ai/tests/unit/test_seed_flipt_flags.py `test_existing_flag_untouched`

### Requirement: The wire model version names generation, tier, ranker and arm

`RecommendResponse.model_version` SHALL be `<generation>/<tier>/<ranker>/<arm>`. The tier SHALL be the ladder strategy that
produced the returned items, `popular` when the ladder floor served, and `serving-fallback` after an unexpected failure. The
ranker SHALL be the one that ranked the items. The `recs.served` log line SHALL carry `tier`, `ranker` and `arm` and no raw id.

#### Scenario: A buyer with precomputed recommendations reports the user tier

- **WHEN** a signed-in buyer who has a precomputed list requests homepage recommendations through the gateway
- **THEN** `model_version` matches `<serving generation>/user_precomputed/<ranker>/<arm>`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `A buyer with precomputed recommendations reports the user tier`

#### Scenario: A similar-items request reports the seed tier and similarity ranker

- **WHEN** a visitor requests similar items for a listing that has vector neighbours
- **THEN** `model_version` matches `<serving generation>/seed_vector_similarity/similarity/none`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `A similar-items request reports the seed tier and similarity ranker`

#### Scenario: The ladder floor reports the popular tier

- **WHEN** every ladder step before the floor yields no candidates and the popular list serves
- **THEN** the tier segment is `popular` and the generation segment is still the serving generation
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_floor_reports_popular_tier`

#### Scenario: A trained ranker is named only when it ran

- **WHEN** a `gbdt` placement is served for a generation that has no trained ranker artifact
- **THEN** the ranker segment is `fixed_weights`, not `gbdt`
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_untrained_gbdt_reports_fixed_weights`

#### Scenario: The served log line carries tier, ranker and arm

- **WHEN** a buyer requests homepage recommendations twice through the gateway
- **THEN** each `recs.served` log line carries `tier`, `ranker` and `arm` equal to the segments of that response's `model_version`, and no raw user id
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature` `The served log line carries tier, ranker and arm`

#### Scenario: An unexpected failure reports the serving-fallback tier

- **WHEN** ranking raises an unexpected error for a request and the popular list serves
- **THEN** the response status is OK and the tier segment is `serving-fallback`
- **VERIFIED BY**: team-ai/tests/unit/test_experiment_arms.py `test_unexpected_failure_reports_fallback_tier`
