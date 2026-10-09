## ADDED Requirements

### Requirement: Batch pipeline produces two-tower vectors

The offline recommendation pipeline SHALL, when the two-tower stage is enabled, train the
two-tower model and load its item vectors into a dedicated vector-store collection as part of the
same run that produces ALS factors. The run summary SHALL report the number of items indexed.

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

### Requirement: Two-tower stage is additive to the ALS baseline

Enabling the two-tower stage SHALL NOT change ALS training, its output contract, or its
collection. With the stage disabled the pipeline SHALL behave exactly as before.

#### Scenario: Disabled stage leaves the ALS run unchanged

- **WHEN** `pipeline.run()` executes with the two-tower stage disabled
- **THEN** the summary matches the ALS-only summary produced before this change
- **AND** no two-tower collection is written

### Requirement: The towers project users and items into one space

Merged from the retired add-two-tower-retrieval. The user tower and the item tower SHALL project user features
(category preferences, activity, lifetime purchases) and item features (category, price, popularity) into one
D-dimensional space. Their vectors SHALL be normalised for cosine similarity.

#### Scenario: User tower and item tower embedding generation

- **GIVEN** user features and item features
- **WHEN** the user tower and the item tower compute embeddings
- **THEN** both vectors have dimension D and unit norm

### Requirement: Two-tower candidates are retrieved by similarity

Merged from the retired add-two-tower-retrieval. Top-K retrieval for a user vector SHALL return item ids ranked by
similarity against the item index.

#### Scenario: Top-K candidate generation for user

- **GIVEN** an item catalog indexed into candidate vectors
- **WHEN** top-K retrieval is requested for a user vector
- **THEN** the top-K item ids are returned ranked by similarity score

