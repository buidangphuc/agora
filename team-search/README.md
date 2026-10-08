# team-search

Search and discovery read-model for the Agora marketplace (Go). It owns the OpenSearch index
`listings`, a projection built by consuming `listing.events` (CQRS, ADR-0005), and the
`platform.search.v1.SearchService` gRPC API: lexical, semantic and hybrid (RRF) listing search,
facets, prefix suggest, and per-user saved searches. Saved searches live in this service's own
Postgres database (`search_db`) when `DATABASE_ENABLED=true`, and in memory otherwise.

Two binaries ship in one image (`Dockerfile`):

| Binary | Entry | Role | Compose service |
|---|---|---|---|
| `/server` (default) | `cmd/server` | gRPC query API on `:50052` | `team-search` (container `team-search-svc`) |
| `/indexer` | `cmd/indexer` | Kafka consumer that writes the index; requires `KAFKA_ENABLED=true` | `team-search-indexer` |

The gateway reaches it through `UPSTREAM_SEARCH_ADDR=team-search-svc:50052`.

## Contract

Source: `proto/platform/search/v1/search.proto` (vendored). Authorization is read from the
gateway-forwarded `x-principal-id` / `x-principal-type` / `x-principal-scopes` metadata
(ADR-0003); this service does not verify JWTs. A missing principal gives `Unauthenticated`, a
missing scope gives `PermissionDenied`.

| RPC | Required scope | Notes |
|---|---|---|
| `SearchListings` | `search:read` | `search_mode` HYBRID / LEXICAL / SEMANTIC; filters, category, price range, min rating, sort; returns hits and facets |
| `Suggest` | `search:read` | Prefix completion over titles; default limit 5, max 20 |
| `SaveSearch` | `search:write` | Needs a non-empty `query` or at least one filter; `filters_json` must be a JSON object of string values |
| `ListSavedSearches` | `search:read` | Caller's own searches, newest first |
| `DeleteSavedSearch` | `search:write` | Owner-scoped; another user's id returns `NotFound` |
| `RunSavedSearch` | `search:read` | Owner-scoped; re-runs the saved query and filters (first page, size 10) |

Behaviour of `SearchListings`:

- Page size default 10, max 50; the cursor is an integer offset.
- Default mode is HYBRID when `ENABLE_HYBRID_SEARCH=true` and the query is non-empty, otherwise LEXICAL.
- Lexical: `multi_match` over `title^2` and `description` plus term, price and rating filters.
- Semantic: embeds the query through modelserve `POST /embed`, then runs a `knn` query on `embedding`.
- Hybrid: runs lexical and semantic concurrently (pool = `from + size`, at least 50, capped at
  `HYBRID_FUSION_WINDOW`), then fuses by RRF: `sum(w_s / (k + rank_s))`, with `k = HYBRID_RRF_K`
  and weights `HYBRID_LEXICAL_WEIGHT` / `HYBRID_SEMANTIC_WEIGHT`.
- Fail-open: the query embed is bounded to 1.5 s. If it errors, times out, or semantic returns
  zero hits, the lexical list is returned. If lexical errors or returns zero hits, the semantic
  list is returned. Both failing returns an error. In SEMANTIC mode an embed or kNN failure
  falls back to lexical.
- Offsets at or beyond `HYBRID_FUSION_WINDOW` skip fusion and use plain lexical paging.
- Optional reranker (`ENABLE_RERANKER=true`): reorders the top 20 fused candidates through
  modelserve `/rerank`; on error the fused order is kept.
- Facets (`categories`, `sellers`, `price_ranges`, `ratings`) are computed over the filtered
  lexical set; see `facetAggs` in `internal/index/opensearch.go` for buckets.

Consumes: modelserve over HTTP (`MODEL_SERVER_URL`: `/embed`, and `/rerank` when enabled). No
upstream gRPC calls.

## Events

| Direction | Topic | Type | Key |
|---|---|---|---|
| Consumes | `listing.events` (`KAFKA_LISTING_TOPIC`), group `team-search-indexer` | `platform.events.v1.EventEnvelope` wrapping `ListingChanged`, `ListingBaseInfoChanged`, `ListingPricingChanged`, `ListingStatusChanged`; other types are ignored | not used by the handler |
| Produces | `listing.events.dlq` (topic + `.dlq`) | the original record, parked after retries are exhausted | original key |

Handling (`internal/consumer/listing.go`):

- Version guard: the document version is the envelope `occurred_at` in nanoseconds; upserts use
  OpenSearch external versioning, so stale or redelivered events are rejected.
- `ListingChanged`: upsert, or delete when `CHANGE_TYPE_DELETED`. Title and description are
  embedded synchronously; on embed failure the document is indexed with `vector_pending=true`.
- `ListingBaseInfoChanged`: partial update of title, description, category, seller, status, and
  re-embed; delete on `CHANGE_TYPE_DELETED`.
- `ListingPricingChanged`: partial update of `price` (promotional price if on sale) and `currency`.
- `ListingStatusChanged`: delete on `REJECTED`, otherwise partial update of `status`.
- Offsets are committed only after a record is handled or parked. Up to 5 attempts per record
  with exponential backoff (100 ms base, 30 s cap), then the DLQ. If the DLQ write fails the
  indexer exits without committing.

## Data

- OpenSearch index `listings` (`OPENSEARCH_INDEX`), owned by this service and rebuildable by
  replaying the topic. `EnsureIndex` creates it at startup in both binaries; the mapping
  (including the 384-dimension `knn_vector`, lucene HNSW, cosine) is in `internal/index/opensearch.go`.
- Postgres `search_db`, table `saved_searches` (`id`, `user_id`, `query`, `filters` JSONB,
  `created_at`; index on `(user_id, created_at DESC)`), from `migrations/0001_saved_searches.{up,down}.sql`.
- Migrations are not applied by the service. In root compose, the `team-search-migrate`
  one-shot (golang-migrate) runs them and `team-search` depends on it. Standalone, run
  golang-migrate against `DATABASE_URL`.
- `DATABASE_ENABLED=true` uses the Postgres repository (pgx; the DB is pinged at startup and
  the server fails to start if it is unreachable). `false` uses an in-memory repository and logs
  a warning: saved searches are lost on restart and not shared between replicas.

## Configuration

Source of truth: `internal/config/config.go`. `make check-env` (`TestEnvExampleInSync`) fails
if `.env.example` and the config structs drift in either direction.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | `local` | `prod`/`production` counts as prod |
| `LOG_LEVEL` | `info` | |
| `LOG_JSON` | `true` | |
| `GRPC_HOST` | `0.0.0.0` | |
| `GRPC_PORT` | `50052` | |
| `GRPC_REFLECTION_ENABLED` | `true` | |
| `SHUTDOWN_GRACE_SECONDS` | `10` | |
| `OPENSEARCH_URL` | `http://localhost:9200` | required non-empty |
| `OPENSEARCH_INDEX` | `listings` | |
| `MODEL_SERVER_URL` | `http://localhost:8100` | modelserve base URL (embed, rerank); 2 s HTTP client timeout |
| `EMBEDDING_DIM` | `384` | Declared but not read by any non-test code; the index mapping hardcodes 384 |
| `ENABLE_HYBRID_SEARCH` | `true` | |
| `ENABLE_RERANKER` | `false` | |
| `HYBRID_FUSION_WINDOW` | `200` | |
| `HYBRID_RRF_K` | `60` | |
| `HYBRID_LEXICAL_WEIGHT` | `1.0` | values <= 0 fall back to 1.0 |
| `HYBRID_SEMANTIC_WEIGHT` | `1.0` | values <= 0 fall back to 1.0 |
| `KAFKA_ENABLED` | `false` | the indexer refuses to start unless true; the server does not use Kafka |
| `KAFKA_BROKERS` | `localhost:9092` | comma-separated |
| `KAFKA_CONSUMER_GROUP` | `team-search-indexer` | |
| `KAFKA_LISTING_TOPIC` | `listing.events` | |
| `DATABASE_ENABLED` | `false` | Postgres vs in-memory saved searches; used by the server only |
| `DATABASE_URL` | `""` | required when `DATABASE_ENABLED=true`; `.env.example` supplies a local value |
| `OTEL_ENABLED` | `false` | |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | `.env.example` sets `http://localhost:4317`; the code default is empty |
| `OTEL_SERVICE_NAME` | `team-search` | |

## Run locally

Whole stack (root of the repo): `docker compose up -d --build`. This starts `team-search-migrate`,
`team-search` and `team-search-indexer` with `DATABASE_ENABLED=true`, OpenSearch and Redpanda.
The root compose files do not define a modelserve service, and `MODEL_SERVER_URL` is not set
for team-search there, so embeds fail and hybrid search degrades to lexical (new documents are
indexed with `vector_pending=true`). To get semantic results, run `platform-modelserve`
(`platform-modelserve/docker-compose.local.yaml`, port 8100) and point `MODEL_SERVER_URL` at it.
No `--profile jobs` is needed for this service.

Standalone:

```bash
docker compose -f docker-compose.local.yaml up -d   # OpenSearch 2.15 on :9200 only
# Redpanda: from platform-core/infra, or use the root stack
cp .env.example .env
make proto                                          # generated/ is gitignored
KAFKA_ENABLED=true make indexer                     # Kafka -> OpenSearch
make server                                         # gRPC on :50052
```

Smoke test (scopes are trusted metadata, so send them yourself):

```bash
grpcurl -plaintext -H 'x-principal-scopes: search:read' -d '{"query":"laptop"}' \
  localhost:50052 platform.search.v1.SearchService/SearchListings
grpcurl -plaintext localhost:50052 grpc.health.v1.Health/Check
```

## Build, test and lint

| Command | What it does |
|---|---|
| `make proto` | `buf generate` into `generated/` (needs `buf`); then `go mod tidy` |
| `make server` / `make indexer` | `go run` the binaries |
| `make test` | `go test ./...` |
| `make check-env` | `.env.example` vs config drift gate |
| `make check` | merge gate: `check-env`, `gofmt -l .`, `go vet ./...`, `go test ./...` |

`go.mod` is Go 1.22; `franz-go` is pinned to v1.18.0 because newer tags require Go 1.25. The
Docker build uses `golang:1.22`.

## Spec and verification

- Feature manifest: `FEATURES.yaml` in this repo (`search.query`, `search.suggest`,
  `search.sort-filtering`, `search.facets`, `search.hybrid-retrieval`), all `status: automated`,
  mapped to feature files in `platform-e2e/tests/e2e/features/` (for example
  `buyer/search_and_discovery.feature`, `search/hybrid_retrieval.feature`).
- Verify coverage: `make -C platform-e2e features-check`; for an OpenSpec change,
  `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes are proposed and archived through OpenSpec (`openspec/changes/<id>`) per the root
  README lifecycle. The saved-search RPCs have no `FEATURES.yaml` entry.

## Gotchas

- `generated/` is gitignored; run `make proto` before building or testing, or the packages do not compile.
- `proto/` is vendored from platform-core (ADR-0001). Never edit it here; change the contract in platform-core and re-vendor.
- Compose passes `DATABASE_*` to the indexer too, but the indexer never opens Postgres.
- The index mapping is created once; changing it (for example the vector dimension) needs a new index and a replay of the topic.
- Hybrid search fails open quietly (plain `log.Printf`); a missing modelserve shows up as lexical-only results, not errors.

## Known gaps

- `EMBEDDING_DIM` is inert (the mapping and `.env.example` agree on 384 only by convention).
- `RunSavedSearch` ignores `search_mode`, the engine and the reranker: it calls the lexical index search directly.
- Root compose has no modelserve and does not set `MODEL_SERVER_URL` for team-search (see Run locally).
- Facets come from the lexical result only, and in hybrid mode `Total` is the larger of the two strategies' totals (an estimate).
- With `DATABASE_ENABLED=false` (local/test only; staging/production refuse to boot), saved searches are in memory only.

## References

- Repo rules and ports: [`../AGENTS.md`](../AGENTS.md); lifecycle: [`../README.md`](../README.md)
- ADRs in `../platform-core/docs/ADR/`: 0001 proto distribution, 0002 async broker, 0003 auth model, 0004 observability, 0005 search read-model, 0011 model serving
