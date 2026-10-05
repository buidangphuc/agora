## ADDED Requirements

### Requirement: Tracking events are exported for offline training

team-analytics SHALL keep its DuckDB warehouse on persistent storage and, when
`PARQUET_EXPORT_PATH` and a positive `PARQUET_EXPORT_INTERVAL_SECONDS` are set, SHALL
periodically export the `tracking_events` table to that path as Parquet. It SHALL
replace the file atomically, so a reader never sees a partial export. An export
failure SHALL NOT stop event consumption.

#### Scenario: The export replaces the file atomically

- **WHEN** an export runs while a previous export file exists
- **THEN** the new data is written to a temporary file and renamed over the old one

#### Scenario: Export disabled by default

- **WHEN** `PARQUET_EXPORT_INTERVAL_SECONDS` is 0 or unset
- **THEN** no export runs

### Requirement: team-ai reads the trained item vectors by listing id

team-ai's Qdrant backend SHALL look up a seed listing by the producer's point id
(uuid5 of the listing id in the shared namespace), and SHALL return the payload
`listing_id` of each hit as the candidate id, never the Qdrant point id.

#### Scenario: Similar items come back as listing ids

- **WHEN** similar items are requested for a listing present in the trained collection
- **THEN** the query uses that listing's uuid5 point id, and every returned candidate id
  is a listing id from the payload

### Requirement: The local stack serves trained recommendations

The local compose stack SHALL provide a runnable training job that reads the exported
tracking events and fills Qdrant and Redis, and team-ai SHALL serve recommendations
from them (`RECS_ENABLED=true`, Qdrant backend).

#### Scenario: Home page shows trained recommendations

- **WHEN** the training job has run against the stack's tracking events and a buyer opens
  the home page
- **THEN** the "Gợi ý cho bạn" row shows product cards for real listings, sourced from
  team-ai

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
be treated as a promotable candidate. The promotion gate SHALL NOT compare metrics produced
under different evaluation protocols.

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
- **THEN** the gate does not compare the two values, and the candidate becomes the champion with
  a reason that names both protocols
