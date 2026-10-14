## MODIFIED Requirements

### Requirement: The training run SHALL evaluate the generation it produced

Every pipeline run SHALL score its training recipe against a temporal holdout drawn from the
same interaction window, using the existing `ModelEvaluator`. It SHALL record the resulting
metrics against that run's `model_version`, stamped with the evaluation protocol. The holdout
SHALL NOT leak into the scored model:
- for each user with at least two distinct listings, the target is the most recently
  discovered listing;
- the evaluation model is trained only on that user's events before the discovery;
- the scored ranking excludes the user's training items.

The published model MAY be trained on every event. A run that cannot produce metrics SHALL NOT
be treated as a promotable candidate. The evaluation protocol SHALL be recorded as the version's tag `lineage.eval_protocol`. When the candidate's and the champion's protocol differ, or the champion has no such tag, the gate SHALL NOT compare metrics: it SHALL judge the candidate on the policy `floors` only, with the reason `eval_protocol changed: <old> -> <new>`.

_Migration note (recsys-auto-train): the protocol rule is kept (decision D2) but is now bounded by the policy floors instead of promoting unconditionally; the champion's protocol is read from `lineage.eval_protocol` (seeded from the Redis meta by `migrate-registry`)._

#### Scenario: A run produces ranking metrics for the generation it trained

- **WHEN** the pipeline completes ALS training over a warehouse with enough interactions to form
  a holdout
- **THEN** the run reports `ndcg@10` and `coverage@10` for that generation
- **AND** the reported metrics are attributed to the run's own `model_version`, not to a
  previous run's

#### Scenario: A run with no usable holdout is not a candidate

- **WHEN** the interaction window yields no test events after the temporal split
- **THEN** the run records that no evaluation was possible
- **AND** no candidate is registered, so the promotion gate is not consulted

#### Scenario: The evaluation model never trains on its targets

- **WHEN** a run evaluates its training recipe
- **THEN** no held-out (user, listing) pair, and none of that user's later events, is in the
  evaluation model's training data
- **AND** the metrics carry the evaluation protocol identifier

#### Scenario: Metrics from another evaluation protocol are not compared

- **WHEN** the incumbent champion's metrics were produced under a different evaluation protocol
- **THEN** the gate does not compare the two values and judges the candidate on the policy floors only, with the reason `eval_protocol changed: <old> -> <new>` naming both protocols (a model with no floors is `promotable`)

### Requirement: A candidate SHALL reach serving only after passing the promotion gate

The pipeline SHALL evaluate the run through the model registry (`evaluate`: registers a version and moves no alias) before any serving artifact is published, and SHALL move `champion` (`apply`) only after the artifacts of that model were published. Vectors and cache entries for a rejected
candidate SHALL NOT replace the generation currently being served, and the previous generation
SHALL remain readable.

_Migration note (recsys-auto-train): `ModelMetadata` and `recs:model:champion` are gone; the bootstrap scenario now depends on the registry having no champion (or on `migrate-registry` having seeded v1)._

#### Scenario: A regressing candidate does not become champion

- **WHEN** a run evaluates below the ALS policy's required improvement over the champion's `ndcg@10`
- **THEN** the candidate is registered as a version tagged `gate.outcome=rejected`
- **AND** the `champion` alias of `recsys-als` still names the previous version

#### Scenario: A rejected candidate leaves the previous generation serving

- **WHEN** a candidate is rejected by the gate
- **THEN** the Qdrant points and Redis keys of the previous generation are unchanged
- **AND** `recs:v1:model_version` still names the previous generation
- **AND** no stale-generation prune has been performed

#### Scenario: The first run bootstraps the registry

- **WHEN** a run completes and the registry has no `champion` for `recsys-als`
- **THEN** that run is promoted and becomes the champion
- **AND** its artifacts are published to Qdrant and Redis

### Requirement: Batch pipeline produces two-tower vectors

The offline recommendation pipeline SHALL, when the two-tower stage is enabled, train the
two-tower model and load its item vectors into a dedicated vector-store collection named for the run's generation
(`<QDRANT_TWO_TOWER_COLLECTION>__<model_version>`) as part of the same run that produces ALS factors, when the `recsys-two-tower` candidate is evaluated `promotable` (a rejected candidate leaves the generation without a two-tower collection). The collection SHALL
be written before the serving pointer moves, so a generation is complete or not visible. The run summary SHALL report the
number of items indexed.

The stage SHALL read only governed inputs: the catalogue and item features from the latest `item_popularity@v1`
featurestore snapshot, user features from the latest `user_activity@v2` snapshot, and the training pairs from the run's own
`als_interactions` dataset. With no snapshot the run SHALL exit 2 before Spark starts and register nothing. Item category and
price are not in those views yet; the towers read them as 0 and nothing substitutes a constant.

The towers SHALL be trained (in-batch softmax over the dataset's pairs) and the run summary SHALL report the first and last
epoch loss. A vector that is all zeros or not finite SHALL NOT be written to the vector store; it SHALL be counted in the
summary, and when no usable vector remains the stage SHALL fail, mark the candidate rejected in the registry and publish nothing.

_Migration note (recsys-auto-train): the two-tower stage is now evaluated on a holdout and gated like ALS (see ADDED requirements); it is no longer published merely because its vectors are non-degenerate._

#### Scenario: Pipeline run indexes two-tower vectors and reports the count

- **WHEN** `pipeline.run()` executes with the two-tower stage enabled over a sample catalog
- **THEN** the returned summary contains `two_tower_items` greater than zero
- **AND** the vector-store loader received that many points in the two-tower collection

#### Scenario: Two-tower vectors carry the run generation

- **WHEN** the two-tower stage loads vectors during a run
- **THEN** every loaded point carries the run's `model_version`
- **AND** points from earlier generations are pruned

#### Scenario: Cold-start item receives a vector that ALS cannot produce

- **WHEN** the catalog contains an item with no recorded interactions
- **THEN** that item has a two-tower vector after the run
- **AND** that item has no ALS factor
- **AND** that vector is non-zero and differs from the vector of an item with different recorded features

#### Scenario: A run trains the towers on the dataset's pairs

- **WHEN** the two-tower stage runs over a dataset with interactions
- **THEN** the summary reports a first-epoch and a last-epoch loss, and the model's metadata records the number of pairs,
  the epochs and the feature snapshots (view, version, file, SHA-256) it trained on

#### Scenario: Missing feature snapshots stop the run

- **WHEN** the two-tower stage is enabled and no `item_popularity` snapshot exists
- **THEN** the job exits 2, its log names `ITEM_FEATURES_DIR`, and no model is registered

#### Scenario: A degenerate vector never reaches the vector store

- **WHEN** the catalog contains an item whose features are all zero and the towers are untrained
- **THEN** the summary counts one refused vector and the two-tower collection has no point for that item

### Requirement: The local stack serves trained recommendations

The local compose stack SHALL provide a runnable training job that reads the exported
tracking events and fills Qdrant and Redis, and team-ai SHALL serve recommendations
from them. The compose job SHALL reach the stack's MLflow service for its registry gate and fails closed when it cannot.

_Migration note (recsys-auto-train): the compose `platform-recsys` job now needs `mlflow` (profile of the infra stack) in addition to qdrant and redis; `trained_recs_local.feature` is re-pointed._

#### Scenario: Home page shows trained recommendations

- **WHEN** the training job has run against the stack's tracking events and a buyer opens
  the home page
- **THEN** the "Gợi ý cho bạn" row shows product cards for real listings, sourced from
  team-ai
