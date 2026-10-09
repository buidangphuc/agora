## Context

See proposal.md for the motivation. Current code as of 2026-10-09:

- **platform-featurestore:** about 290 lines of an unused library.
  - `UserFeatures` and `ItemFeatures` dataclasses.
  - An online store over an injected Redis client, using keys `fs:u:` and `fs:i:`.
  - An in-memory offline store.
  - `validate_parity`.
  - The Dockerfile only runs pytest. The package reads no env, and the README calls it unused.
- **team-analytics:**
  - `internal/export` writes `tracking_events` to `PARQUET_EXPORT_PATH` every `PARQUET_EXPORT_INTERVAL_SECONDS` (300
    locally), using an atomic rename.
  - DuckDB holds an exclusive lock, so other processes can only read the export.
  - Compose mounts the `analytics_data` volume read-only into the recsys job.
- **platform-recsys:** reads `/data/tracking_events.parquet` with Spark. It is the precedent for a batch job on the
  analytics volume. The e2e suite runs that image with `tests/e2e/flows/recsys_job_driver.py`.

## Goals / Non-Goals

**Goals:**
- One SQL definition per feature view, computed point-in-time.
- An offline snapshot and an online copy, proven to agree.
- Re-runnable at any `AS_OF`.

**Non-Goals:** serving to team-ai, training datasets, scheduling, and a gRPC service.

## Decisions

### D1. DuckDB in-process over the Parquet exports
- The job runs DuckDB in Python, `read_parquet` over `FEATURESTORE_INPUT_DIR/*.parquet`.
- It needs no Spark, starts in seconds, and runs the same SQL in e2e and CI.
- Alternative considered: Spark, like recsys. Rejected for this job size; it can be reconsidered when the data outgrows
  one node.

### D2. The registry
`registry/features.yaml` holds a list of views. Each view has:

| Field | Meaning |
|---|---|
| `name` | View name |
| `version` | Integer version |
| `entity` | `user_key` or `listing_id` |
| `sql` | Path to a `.sql` file |
| `features` | `{name: type}` |

- The SQL receives `$as_of` (TIMESTAMP) and reads the views `events` (`tracking_events_resolved` filtered by
  `ingested_at <= $as_of`), `facts` (`engagement_facts` filtered the same way) and `orders` (`order_facts` filtered by
  `occurred_at <= $as_of`). The job creates these filtered views, so no definition can bypass the point-in-time rule.
- The definition hash is the SHA-256 of the SQL text plus the YAML entry.
- A view whose definition hash changes without a version bump fails the job. A unit-test-level guard also checks the
  committed registry against a hash lock file `registry/features.lock`.

### D3. The point-in-time rule
- **Windows:** `occurred_at > $as_of - INTERVAL 7 DAY AND occurred_at <= $as_of` for 7-day counts. There is no
  30-day order window in v1, because `order_facts` has no buyer.
- **Current state:** the facts with `occurred_at <= $as_of` are reduced per pair, with the same latest-fact rule as the
  warehouse views. The warehouse views themselves are not used, because they are unfiltered.
- **Default:** `AS_OF` is `now()` in UTC. An explicit `AS_OF` must be RFC 3339.

### D4. Outputs
- **Offline snapshot.** `<FEATURESTORE_OFFLINE_DIR>/<view>/v<ver>/as_of=<YYYYMMDDTHHMMSSZ>.parquet`, written to `.tmp`
  and then renamed.
- **Manifest.** `manifest.json` in `<FEATURESTORE_OFFLINE_DIR>/runs/<as_of>/` holds `as_of`, `materialized_at`,
  `input_watermark`, `views: [{name, version, rows, definition_sha256, snapshot}]`, and the input files with their size
  and mtime.
- **Online store.** A Redis pipeline with `SET fs:<view>:v<ver>:<id> <json> EX ttl` per entity. Then
  `SET fs:<view>:current <ver>` and `SET fs:<view>:meta <json>`, written last so readers never see a pointer to a
  half-written version.
- **Prototype replaced.** The old `fs:u:`/`fs:i:` API and the dataclasses are replaced by a generic row writer. The
  dataclass shapes were never used. `validate_parity` is kept and generalised to rows.

### D5. Parity
- The job samples up to `FEATURESTORE_PARITY_SAMPLE` entity ids per view, chosen deterministically (lowest hash of
  id plus `as_of`).
- It reads them back from Redis and compares each feature with `math.isclose(rel_tol=abs_tol=1e-9)`, or equality for
  non-numeric values.
- A mismatch exits with code 3 and prints `parity mismatch view=<v> entity=<id> feature=<f> online=<x> offline=<y>`.
- `parity` without `materialize` reads the latest manifest and its snapshots.

### D6. Packaging and compose
- **Image.** The Dockerfile becomes `python:3.12-slim` with duckdb, pyarrow and redis, and the entrypoint
  `python -m featurestore`. The tests run in a separate stage.
- **Compose.** Service `featurestore-job`, `profiles: [featurestore]`:
  - mounts `analytics_data:/data:ro` and `featurestore_data:/features`;
  - `FEATURESTORE_REDIS_URL=redis://redis:6379/2`, its own DB index;
  - `command: materialize`.
- **E2E.** The tests run it with `docker run --rm --network <stack> -v ...` like the recsys driver, passing `AS_OF` as
  needed.

### D7. ADR-0015
The ADR records:
- decision D1 of 2026-10-08;
- the inputs contract (the export file names);
- the key scheme;
- the point-in-time rule;
- the alternative left open (a gRPC online read later);
- a supersede note for the README's old team-analytics pointer.

## Risks / Trade-offs

- **[The export lags up to `PARQUET_EXPORT_INTERVAL_SECONDS` (300 s locally)]** Mitigation: features are at most one
  export behind. e2e waits for an export cycle, or a test can lower the interval in an overlay.
- **[Redis memory grows with entities × versions]** Mitigation: the TTL is 2 days, and an old version expires after
  a bump.
- **[DuckDB reading a Parquet file during rename]** Mitigation: the rename is atomic, and the job retries a read once on
  an I/O error.

## Migration Plan

The work is additive. Deploy team-analytics (more export files) first, then use the job. Rollback removes the profile
service. The old prototype API had no users.
