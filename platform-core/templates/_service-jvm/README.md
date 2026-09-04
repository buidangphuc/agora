# _service-jvm — JVM/Kotlin gRPC service seed (Hexagonal Architecture)

A **seed template**, not a finished service. It's the skeleton the enterprise
domain services (`team-domain`: listing / billing / user) are cut from. It
compiles into a shape you fill in; the gRPC adapter needs generated proto code
before it builds (`make proto`).

## Hexagonal architecture (ports & adapters)

The dependency arrow points **inward**. Domain knows nothing about the outside;
adapters depend on the domain, never the reverse.

```
        inbound adapters                 domain (pure)                 outbound adapters
   ┌───────────────────────┐      ┌───────────────────────┐      ┌───────────────────────┐
   │ infrastructure/grpc   │      │ domain/model          │      │ infrastructure/db     │
   │  ListingGrpcService   │─────▶│  Listing, Principal…  │◀─────│  InMemoryListing-     │
   │  AuthInterceptor      │ port │ domain/port           │ port │  Repository (stub)    │
   │  TracingInterceptor   │      │  ListingRepository    │      │  → Postgres later     │
   └───────────────────────┘      │  GetListingPort       │      └───────────────────────┘
                                   └───────────┬───────────┘
                                               │ orchestrated by
                                   ┌───────────▼───────────┐
                                   │ application           │
                                   │  GetListingUseCase    │
                                   └───────────────────────┘
```

- **`domain/`** — pure business. `model/` (data classes + enums) and `port/`
  (interfaces: driven `ListingRepository`, driving `GetListingPort`). **No
  gRPC, proto, or JDBC imports here.** This is the rule that keeps the core
  testable and swappable.
- **`application/`** — use-cases orchestrating the domain through ports
  (`GetListingUseCase`). Also framework-free.
- **`infrastructure/`** — adapters that implement/drive the ports. `grpc/` is
  the inbound side (translates proto ↔ domain, maps errors, resolves identity);
  `db/` is the outbound side (persistence).
- **`Server.kt`** — the composition root: constructs adapters, injects them into
  use-cases, assembles the gRPC server. The only place that sees every layer.

## Instantiate this template

1. **Rename** `team-service` → your service name (e.g. `listing-service`):
   `settings.gradle.kts`, `build.gradle.kts` (group), `Dockerfile`,
   `.env.example` (`OTEL_SERVICE_NAME`).
2. **Pin the proto submodule** under `proto-vendor/` at a tag — see
   [`proto-vendor/README.md`](proto-vendor/README.md) (placeholder `proto/v0.1.0`).
3. **`make proto`** — generate gRPC/proto code into `./generated`. The gRPC
   adapter (`infrastructure/grpc/ListingGrpcService.kt`) and `Server.kt`'s
   service registration compile only after this step.
4. **`cp .env.example .env`**, then **`make run`** — starts the server on
   `GRPC_PORT` (default `50053`) with health (SERVING) + reflection, so you can
   probe it with `grpcurl`.
5. **`make check`** — the local merge gate (ktlint + tests). Green before PR.

## Rules (platform conventions)

- **Don't edit `generated/`.** It's produced by `make proto`, gitignored, and
  regenerated on demand. Change the proto contract upstream and re-pin (ADR-0001).
- **Generated JVM types live under `com.platform.*`.** buf managed mode adds the
  default `com` java package prefix, so proto package `platform.listing.v1`
  generates into `com.platform.listing.v1` (and `platform.common.v1` into
  `com.platform.common.v1`). Only the adapters import these; the domain and
  application layers stay framework/proto-free.
- **Keep the domain framework-free.** No gRPC/proto/JDBC imports under `domain/`
  or `application/`. Translation lives in the adapters.
- **Own your DB only.** DB-per-service: this service touches `listing_db` and
  nothing else. Migrations are Flyway (`src/main/resources/db/migration`).
- **Cross-service calls go over gRPC**, never a shared database or shared tables.
- **Auth & tracing are interceptor concerns** (ADR-0003 / ADR-0004): identity is
  a resolved `Principal` in the Context; services never parse credentials.

## Layout

```
build.gradle.kts / settings.gradle.kts   Gradle (Kotlin JVM, generated/ source set)
buf.gen.yaml                             buf codegen → ./generated (java+kotlin+grpc)
Makefile                                 proto | run | check | test | migrate
Dockerfile                               multi-stage, non-root JRE runtime
.env.example                             GRPC_PORT, DATABASE_URL, OTEL_*, bearer
proto-vendor/README.md                   submodule pin + upgrade steps (ADR-0001)
src/main/kotlin/
  Server.kt                              composition root + main()
  domain/model/                          Listing, ListingStatus, Principal, errors
  domain/port/                           ListingRepository, GetListingPort
  application/                           GetListingUseCase
  infrastructure/grpc/                   ListingGrpcService, Auth/Tracing interceptors
  infrastructure/db/                     InMemoryListingRepository (stub)
src/main/resources/db/migration/         Flyway V1__init.sql
```

## Referenced ADRs

- **ADR-0001** — proto distribution (pin + local-generate).
- **ADR-0003** — auth model (`Principal` + scopes; JWT/cookie still open).
- **ADR-0004** — observability (OpenTelemetry, exporter-swappable).
