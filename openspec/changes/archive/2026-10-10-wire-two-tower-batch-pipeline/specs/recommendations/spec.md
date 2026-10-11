## ADDED Requirements

### Requirement: Batch pipeline produces two-tower vectors

The offline recommendation pipeline SHALL, when the two-tower stage is enabled, train the
two-tower model and load its item vectors into a dedicated vector-store collection named for the run's generation
(`<QDRANT_TWO_TOWER_COLLECTION>__<model_version>`) as part of the same run that produces ALS factors. The collection SHALL
be written before the serving pointer moves, so a generation is complete or not visible. The run summary SHALL report the
number of items indexed.

The stage SHALL read only governed inputs: the catalogue and item features from the latest `item_popularity@v1`
featurestore snapshot, user features from the latest `user_activity@v2` snapshot, and the training pairs from the run's own
`als_interactions` dataset. With no snapshot the run SHALL exit 2 before Spark starts and register nothing. Item category and
price are not in those views yet; the towers read them as 0 and nothing substitutes a constant.

The towers SHALL be trained (in-batch softmax over the dataset's pairs) and the run summary SHALL report the first and last
epoch loss. A vector that is all zeros or not finite SHALL NOT be written to the vector store; it SHALL be counted in the
summary, and when no usable vector remains the stage SHALL fail, reject the candidate and publish nothing.

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

### Requirement: Two-tower stage is additive to the ALS baseline

Enabling the two-tower stage SHALL NOT change ALS training, its output contract, or its
collection. With the stage disabled the pipeline SHALL behave exactly as before.

#### Scenario: Disabled stage leaves the ALS run unchanged

- **WHEN** `pipeline.run()` executes with the two-tower stage disabled
- **THEN** the summary matches the ALS-only summary produced before this change
- **AND** no two-tower collection is written

### Requirement: The towers project users and items into one space

Merged from the retired add-two-tower-retrieval. The user tower and the item tower SHALL project user features
(category preferences, activity, lifetime purchases) and item features (category, price, click-through rate, popularity)
into one D-dimensional space. Their vectors SHALL be normalised for cosine similarity.

#### Scenario: User tower and item tower embedding generation

- **GIVEN** user features and item features
- **WHEN** the user tower and the item tower compute embeddings
- **THEN** both vectors have dimension D and unit norm
- **VERIFIED BY**: platform-recsys/tests/test_two_tower.py › test_towers_projection_and_normalization. Not verifiable end to end: the projection is an in-process function; no deployed surface takes a feature dict and returns an embedding.

### Requirement: Two-tower candidates are retrieved by similarity

Merged from the retired add-two-tower-retrieval. Top-K retrieval for a user vector SHALL return item ids ranked by
similarity against the item index.

#### Scenario: Top-K candidate generation for user

- **GIVEN** an item catalog indexed into candidate vectors
- **WHEN** top-K retrieval is requested for a user vector
- **THEN** the top-K item ids are returned ranked by similarity score
- **VERIFIED BY**: platform-recsys/tests/test_two_tower.py › test_top_k_is_ranked_by_similarity_and_bounded. Not verifiable end to end: `TwoTowerModel.retrieve` is in-process and this change has no serving surface (serving-side blending is the placement engine's, a non-goal).
