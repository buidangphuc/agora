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
