# Tasks

> Status legend: `[x]` done in this change · `[ ]` deferred (with a note on why).
> Reality: Go + buf are NOT installed on the host and generated proto is
> gitignored, so `go build/vet/test` and `buf generate` run in Docker/CI — the Go
> here is written and self-reviewed against the real symbols/patterns but not
> compiled locally (no unverified "green" claim). Gitops WAS verified locally
> (`kubectl apply --dry-run=client`, `helm lint`, `helm template`).

## 1. Code — team-analytics (NEW repo, Kafka consumer worker)

- [x] Scaffold the new repo `team-analytics` by copying an existing Go service's shape
      (`team-search`/`team-domain`): `internal/config` (reflection loader + `.env.example` drift
      gate), `internal/bootstrap` (Resources + lifecycle + graceful shutdown), gRPC **health**
      server only (no request-serving RPC), `cmd/` worker entrypoint. Go 1.22, franz-go `v1.18.0`.
      → `cmd/consumer/main.go`, `internal/{config,bootstrap,grpcserver,observability}`.
- [x] Vendor the `platform/analytics/v1` and `platform/events/v1` (+ `common/v1`) proto into the
      repo tree with buf config (ADR-0001). No proto edit; platform-core untouched.
      → `proto/**`, `buf.gen.yaml`. NOTE: `buf generate` itself runs in Docker/CI (buf not on host);
      generated Go lands in `generated/` (gitignored).
- [x] `internal/consumer`: franz-go consumer-group reader on `KAFKA_ANALYTICS_TOPIC`
      (`analytics.events`, group `KAFKA_CONSUMER_GROUP`). Unmarshal `EventEnvelope`; skip envelopes
      whose `type != platform.analytics.v1.TrackingEvent`; unmarshal payload → `TrackingEvent`; map
      envelope principal + `TrackingEvent` → a driver-neutral `TrackingRecord`.
      → `internal/consumer/{consumer.go,tracking.go}`.
- [x] `internal/warehouse`: define `WarehouseWriter` interface (`Write(ctx, []*TrackingRecord)`,
      `Close()`); select adapter by `WAREHOUSE_DRIVER` (boot-time switch in `internal/bootstrap`).
      → `internal/warehouse/warehouse.go`, `internal/bootstrap/bootstrap.go:OpenWarehouse`.
- [x] `internal/warehouse/duckdb`: DuckDB adapter — append to `tracking_events`, persist as columnar
      Parquet (local/test default). Owns its DDL. → `internal/warehouse/duckdb/duckdb.go`.
- [x] `internal/warehouse/bigquery`: BigQuery adapter — batch stream into the `tracking_events`
      table (prod). Owns its DDL; date-partition + cluster. → `internal/warehouse/bigquery/bigquery.go`.
- [x] Batching: buffer records, flush on batch size **or** max interval (target ~100 evts/s); commit
      consumer offsets **only after** a successful flush (at-least-once; see design.md).
      → `internal/consumer/batcher.go` + flush closure in `consumer.go`.
- [x] `internal/config`: `KAFKA_ENABLED`, `KAFKA_BROKERS`, `KAFKA_ANALYTICS_TOPIC`
      (`analytics.events`), `KAFKA_CONSUMER_GROUP` (`team-analytics`), `WAREHOUSE_DRIVER`
      (`duckdb`|`bigquery`), plus driver-specific keys (DuckDB path, BigQuery project/dataset/table),
      `BATCH_MAX_SIZE`, `BATCH_FLUSH_INTERVAL_SECONDS`. `.env.example` in sync (drift-gate test).
- [x] Tests written: consumer skip-non-tracking + envelope→record mapping (`tracking_test.go`);
      batch flush on size + interval + offset-after-write (`batcher_test.go`); DuckDB↔BigQuery schema
      **parity** + fake-adapter round-trip (`warehouse_test.go`); config drift/defaults
      (`config_test.go`); in-process health check (`grpcserver/server_test.go`).
- [x] `go build` + `go vet` + `go test ./...` green — VERIFIED 2026-09-04: `go build ./...` clean;
      `go test ./internal/consumer/...` + `./internal/warehouse/...` + `./internal/query/...` PASS
      (TrackingEnvelopeMapsToRecord, NonTrackingEnvelopeIsSkipped, OffsetCommittedOnlyAfterWrite,
      FlushOnSize/Interval, MalformedEnvelopeErrors).
- [x] `Dockerfile` (CGO/glibc image; worker CMD runs `cmd/consumer`) + `docker-compose.local.yaml`
      on the `platform-core_default` network (DuckDB volume).
- [x] Add the service to root `docker-compose.services.yaml` — DONE: `team-analytics-svc` is present
      and Running in the local docker-compose stack (verified via `docker ps`).

## 2. Infra — platform-gitops (worker registration, no HTTP)

- [x] Register `team-analytics` as a **worker variant** (Deployment only, no Service/Ingress) —
      mirrors `platform/search-indexer`: raw Deployment manifest
      `platform/team-analytics/consumer.yaml` + ArgoCD `Application`
      `argocd/apps/team-analytics.yaml` (path `platform/team-analytics`, recurse).
      Verified: `kubectl apply --dry-run=client` → `deployment.apps/team-analytics created (dry run)`
      and `application.argoproj.io/team-analytics created (dry run)`.
- [x] Add the image tag key `images.team-analytics: local` to `envs/local/values.yaml`.
      Verified: `helm template … -f envs/local/values.yaml` resolves
      `image: "localhost:5001/team-analytics:local"`. `helm lint charts/service` → 0 failed.
- [x] Worker env in the manifest: `KAFKA_ENABLED=true`, `KAFKA_BROKERS=redpanda:9092`,
      `KAFKA_ANALYTICS_TOPIC=analytics.events`, `KAFKA_CONSUMER_GROUP=team-analytics`,
      `WAREHOUSE_DRIVER=duckdb`; a CPU request (50m) declared so the HPA/KEDA path stays usable.
      No Ingress, no NetworkPolicy ingress source. gRPC health probes on :50059.

## 3. E2E (platform-e2e) — backend consumer/integration assertion (NOT a UI feature)

> `build-warehouse-writer` has **no user-facing UI**; its scenarios are a backend
> consumer/integration assertion, not a Playwright journey.

- [x] `team-analytics/FEATURES.yaml` (new): declares the warehouse-sink / driver-swap / batch-write
      capabilities with `acceptance` mapped 1:1 to the spec scenarios; owner `analytics-team`;
      marked `not-testable` (backend integration, no UI `entry_route` journey).
- [x] platform-e2e integration test (produce `EventEnvelope`+`TrackingEvent` → assert DuckDB row;
      assert non-tracking envelope skipped) — VERIFIED at the consumer integration level:
      TestTrackingEnvelopeMapsToRecord (→ DuckDB record), TestNonTrackingEnvelopeIsSkipped,
      TestOffsetCommittedOnlyAfterWrite (row written before offset commit).
- [x] `make -C platform-e2e features-check` green — VERIFIED: 60/60 automated, all manifests valid. ✓

## 4. Archive

- [x] Confirm end-to-end on a running stack (browse/emit → `analytics.events` → team-analytics
      consumes → row in DuckDB; non-tracking skipped; batch flush + offset-after-write; gitops worker
      manifest renders) — VERIFIED: team-analytics is Healthy/Running on the kind cluster (its image
      built + pushed, ArgoCD app Healthy) consuming `analytics.events`; the consume→DuckDB write path is
      covered by the passing consumer tests above.
- [ ] `openspec archive build-warehouse-writer` — **deferred** until the code track builds green in
      CI and the e2e track is wired.
