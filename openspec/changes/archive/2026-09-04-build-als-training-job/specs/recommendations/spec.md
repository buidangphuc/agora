## ADDED Requirements

### Requirement: An offline batch job trains ALS from the behavioral warehouse

The platform SHALL provide a `platform-recsys` PySpark batch job that reads the behavioral
warehouse (`tracking_events` — DuckDB/Parquet local, BigQuery prod), maps rows over a rolling
interaction window to implicit-feedback `(user, item, weight)` triples (user = `principal_id` when
present else `anonymous_id`; item = `listing_id`; weight derived from `event_type`), and fits a
Spark MLlib ALS model with `implicitPrefs=true`. The job SHALL read only the warehouse and write
only its own artifact stores (Rule 3), and SHALL run as a scheduled offline batch — not on the
request path.

#### Scenario: The job trains a model from sample warehouse data

- **WHEN** the job runs against a sample `tracking_events` dataset containing view/click/add-to-cart
  events for several users and listings
- **THEN** it produces an ALS model with item factors and user factors, having skipped rows with an
  empty `listing_id` and collapsed events to weighted per-(user,item) interactions

### Requirement: The job publishes item and user vectors to Qdrant

The job SHALL load the ALS **item factors** and **user factors** into Qdrant (`:6333`, the instance
`team-ai` already targets) as two collections — item vectors (for item-item similarity) and user
vectors (for personalized "for-you") — each point payload carrying the source id and the
`model_version` of the run, so the online serving layer can nearest-neighbor query them.

#### Scenario: A training run populates the Qdrant collections

- **WHEN** the job finishes training on the sample dataset and loads its outputs
- **THEN** the item-vector and user-vector Qdrant collections exist and are populated with one point
  per trained listing / user, each stamped with the run's `model_version`
  <!-- backend integration assertion: job runs on sample warehouse data → Qdrant collection populated -->

### Requirement: The job writes a precomputed recommendation cache to Redis

The job SHALL write a precomputed top-N recommendation cache into Redis (`:6379`): per-user ranked
recommendations, per-listing similar items, and the current `model_version`, with a TTL longer than
the batch cadence so a missed run degrades gracefully rather than emptying the cache. The cache
SHALL hold listing ids and scores only (no hydrated listing content, Rule 3).

#### Scenario: A training run populates the Redis cache

- **WHEN** the job finishes loading its outputs
- **THEN** Redis holds a per-user recommendation entry and a per-listing similar-items entry (ranked
  listing ids + scores) plus a model-version key identifying the generation just written

### Requirement: The job runs locally without a Spark cluster

The job SHALL run in Spark local mode (no cluster) reading the DuckDB-exported Parquet warehouse and
writing to local Qdrant/Redis from `platform-core`'s compose, so the full train→load loop is
runnable on a developer laptop and in CI on a small sample dataset.

#### Scenario: A developer runs the job end-to-end locally

- **WHEN** a developer runs the job in local mode against the sample Parquet warehouse with local
  Qdrant (`:6333`) and Redis (`:6379`) running
- **THEN** the job completes without a Spark cluster and leaves the Qdrant collections and Redis
  cache populated for that sample
