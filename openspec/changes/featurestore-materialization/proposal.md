## Why

This is AI-first change 4 of 7.

The recommenders need per-user and per-item features computed from the warehouse. agora has none:
- `platform-featurestore` is an unused library whose stores are in-memory dicts.
- Nothing materialises features.
- team-ai wires an `InMemoryFeatureStore`.

On 2026-10-08 the user decided (D1) that features are computed by `platform-featurestore` jobs that read the
warehouse's Parquet exports. They are not computed inside team-analytics. Changes 1–3 now give those jobs clean inputs:
- `tracking_events_resolved`, with a stitched `user_key` and `ingested_at`;
- `engagement_facts`;
- `order_facts`.

## What Changes

- **team-analytics:** the Parquet exporter also writes `tracking_events_resolved.parquet`, `engagement_facts.parquet`
  and `order_facts.parquet`. Each file is replaced atomically, beside the existing `tracking_events.parquet`.
- **platform-featurestore:** becomes a batch job, `python -m featurestore materialize`.
  - **Registry.** A versioned registry (`registry/features.yaml`) declares two feature views, `user_activity@v1` and
    `item_popularity@v1`. Each is one SQL definition over the Parquet inputs.
  - **Point in time.** The job computes every view as of a time `AS_OF`, which defaults to now. It uses only rows
    ingested at or before `AS_OF`, and windows are relative to `AS_OF`.
  - **Offline output.** A Parquet snapshot per view, version and `as_of`, plus a `manifest.json` per run. The manifest
    holds `as_of`, the row counts, the definition hash and the input watermark.
  - **Online output.** Redis keys `fs:<view>:v<version>:<entity_id>`, with a TTL, plus `fs:<view>:current` (the
    version) and `fs:<view>:meta` (`as_of`, `materialized_at`, the input watermark).
  - **Parity gate.** After writing, the job compares a sample of online values with the offline snapshot, and fails on
    any mismatch. `python -m featurestore parity` runs the same check on its own.
- **Compose:** a `featurestore-job` service under the `featurestore` profile. It mounts the analytics volume read-only,
  plus its own `featurestore_data` volume and Redis.
- **platform-core:** ADR-0015 records decision D1, the key scheme and the point-in-time rule.
- **platform-featurestore README:** it drops the "unused prototype" status and the old pointer to a team-analytics
  feature store.
- **platform-e2e:** scenarios run the real job image against the stack.

Repos touched: team-analytics, platform-featurestore, platform-core (ADR), root compose, platform-e2e. **No proto
change.**

## Capabilities

### New Capabilities
- `feature-materialization`: the feature registry, the point-in-time materialisation, the offline snapshot and
  manifest, the online keys, and the parity gate.

### Modified Capabilities
- None.

## Non-goals

- Serving features to team-ai, or changing ranking. That is `recs-serving-safeguards` (change 7).
- Training datasets built from these features. That is `featurestore-datasets` (change 5).
- A scheduler. The job runs on demand (compose profile, e2e, CI). A CronJob in gitops is a follow-up once a deployed
  environment needs it.
- A gRPC feature service. The design decision kept online reads as a library over Redis for now.

## Impact

- **New featurestore settings:**
  - `FEATURESTORE_INPUT_DIR` (default `/data`)
  - `FEATURESTORE_OFFLINE_DIR` (default `/features`)
  - `FEATURESTORE_REDIS_URL`
  - `FEATURESTORE_ONLINE_TTL_SECONDS` (default 172800)
  - `FEATURESTORE_PARITY_SAMPLE` (default 200)
  - `AS_OF`
- **team-analytics:** no new setting. The export writes four files instead of one when `PARQUET_EXPORT_PATH` is set.
  The extra files sit beside it.
- **Redis key scheme:** the unversioned `fs:u:`/`fs:i:` keys of the old prototype are replaced. Nothing reads them.
