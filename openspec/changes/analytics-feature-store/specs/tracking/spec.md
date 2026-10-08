## MODIFIED Requirements

### Requirement: The warehouse target is swappable behind a WarehouseWriter seam

The worker SHALL write through a single `WarehouseWriter` interface. DuckDB SHALL be the primary warehouse in
every environment, because the feature store computes on it; `WAREHOUSE_DRIVER=duckdb` remains the only
accepted primary driver. BigQuery SHALL be an optional additional sink for business intelligence, enabled by
`WAREHOUSE_BI_SINK=bigquery`, receiving the same rows after the primary write succeeds. A BigQuery failure
SHALL NOT block the primary write or the Kafka offset commit; it SHALL be counted and retried. Adding or
removing the BI sink SHALL require no change to the consume/unmarshal path, only the env value. Every row SHALL
carry the time the worker received it (`ingested_at`) in addition to the producer's occurrence time.

#### Scenario: Local runs use the DuckDB adapter

- **WHEN** the worker starts with `WAREHOUSE_DRIVER=duckdb` and no BI sink
- **THEN** it writes tracking rows, each with an `ingested_at` time, to the local DuckDB warehouse only

#### Scenario: The BigQuery BI sink receives the same rows

- **WHEN** the worker starts with `WAREHOUSE_BI_SINK=bigquery`
- **THEN** every row written to DuckDB is also streamed to BigQuery with the same columns, and event handling
  is unchanged

#### Scenario: A BigQuery outage does not stop ingestion

- **WHEN** the BigQuery sink fails while the BI sink is enabled
- **THEN** rows are still written to DuckDB, Kafka offsets still advance after the DuckDB write, and a
  BI-sink failure counter increases

#### Scenario: BigQuery is refused as the primary driver

- **WHEN** the worker starts with `WAREHOUSE_DRIVER=bigquery`
- **THEN** it refuses to start with a message that BigQuery is a BI sink only (`WAREHOUSE_BI_SINK`)
