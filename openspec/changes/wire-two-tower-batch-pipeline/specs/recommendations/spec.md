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
