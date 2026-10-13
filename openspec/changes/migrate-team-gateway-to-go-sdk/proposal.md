## Why

team-gateway is the last Go service on its own platform layer (ADR-0016, phase 6). It is a Connect edge, not a gRPC service, so only the platform parts of go-sdk apply. It still carries its own copies of the config binder and envcheck, the tracer, meter and logging (`internal/observability`), and its own lifecycle in a 243-line `cmd/gateway/main.go`.

initiative: go-platform-sdk

## What Changes

- Vendor `go-sdk/v0.2.1`.
- Adopt the SDK platform parts that fit an edge:
  - SDK `config` groups: `Runtime`, `OTel`, `Kafka`, plus the gateway's own groups, keeping every env name;
  - `logx`;
  - `otelx` (tracer and meter; the gateway's RED metrics keep their names);
  - `lifecycle` (the HTTP server, the revocation consumer and the analytics publisher as components, with drain-before-stop);
  - `healthx`, if the edge exposes health;
  - `kafkax`, for its producer and consumer clients;
  - `upstream`, only if it fits the Connect clients. Otherwise keep `internal/upstream` and say why.
- Add `internal/app` as the composition root. `cmd/gateway/main.go` becomes signal context → `config.Load` → `app.Run`.
- Remove `KAFKA_ENABLED` only if every deploy runs it on. `EDGE_REFLECTION_ENABLED` and `OTEL_ENABLED` are real toggles and stay: reflection must be off in prod, and OTel is on only in staging.
- **Not adopted:**
  - `grpcx`, because the edge serves Connect/HTTP;
  - `auth` interceptors, because the gateway verifies the JWT itself and forwards `x-principal-*`.

No route, policy, CORS, rate-limit or JWT behaviour changes.

## Non-goals

Edge policy changes, Connect transport changes, and frontend changes.

## Impact

- Repo: `team-gateway`, its compose block, and `platform-gitops/{argocd/apps,envs/services,envs/staging/kind}/team-gateway.yaml`.
- Capability: `gateway-runtime` (new).

## Non-functional requirements

- No regression in about 107 existing tests.
- Shutdown drains in-flight HTTP requests within the grace period.

## Data classification

internal/personal, unchanged (JWT claims, principal ids). No new data is collected.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Edge auth | Spoofing via a regression in JWT verification | Unchanged code path; existing auth tests keep their assertions |
| Reflection | Information disclosure in prod | `EDGE_REFLECTION_ENABLED` validation kept |
