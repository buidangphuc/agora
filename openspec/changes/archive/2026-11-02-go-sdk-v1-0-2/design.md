## D1 Reflection stays a flag, defaults off, refused in strict ENV

The plan lists `GRPC_REFLECTION_ENABLED` as a kept debug flag, not a dependency flag. It lives in `config.GRPC` (`ReflectionEnabled bool`, `env:"GRPC_REFLECTION_ENABLED" default:"false"`), so `grpcx.ServerOptions.Config` already carries it. `GRPC` gets a pointer-receiver `ResolveFor(Runtime)`, found by the existing `resolveAll` pass, so the refusal runs before `Validate` like `MockData` does (an operator sees the real cause, not a port error). Rejected: a `ServerOptions.Reflection` field (the strict-ENV refusal would then be each service's job) and building reflection only in local code (a service needs it from a settings value).

## D2 `liveness` is a second name on the same health server

`healthx.New` calls `server.SetServingStatus("liveness", SERVING)`. `apply` only touches `""`, so checks cannot affect `liveness`. `Drain` already calls `server.Shutdown()`, which sets every registered name NOT_SERVING and ignores later updates, so `liveness` goes NOT_SERVING exactly when readiness does, only at shutdown. Rejected: a second `health.Server` (two registrations, two things to drain) and an HTTP `/livez` (ADR-0016 services serve gRPC only).

## D3 Chart value is opt-in and touches liveness only

K8s gRPC probes send the `service` field to `Check`. Readiness and startup keep the empty name, so startup still waits for dependencies and readiness still gates traffic. The value defaults to `""`; a non-SDK service has no `liveness` name, and probing it would return NOT_FOUND and kill the pod, so the default must stay empty and each service sets it in its own values file when it adopts the SDK (a per-service-migration task, not here). The helm `with`/`if` guard keeps the empty render byte-identical. Both the Deployment and the Rollout use `service.container`, so one edit covers both.

## D4 Verification of the chart

`platform-gitops` has no helm test. The existing static check `platform-e2e/tests/e2e/features/ops/gitops_render.feature` already runs `helm template` on the real chart; the four chart scenarios are added there, with `--set` overrides in the step (a small extension of `_render`), rather than a new harness.

## D4b Golden render

The default-render scenario compares with a render of the pre-change chart stored as a fixture, so "byte-identical" is checked against the old template, not against the new one with an empty value.

## D5 Removal of OpenOrLocal needs no replacement

The nil-pool contract existed only for services that ran without a database locally. Those services get a database from compose; tests use `testkit`. No in-memory substitute is added to the SDK.

## Clarifications

### Round 1 — 2026-10-22 (human)
- v1.0.2 removes `postgres.OpenOrLocal`, makes gRPC reflection off by default (refused in strict ENV), and adds the `liveness` health service with the opt-in chart value `probes.livenessService`.
- The user chose to do this before migrating sharing and referral, so those services adopt v1.0.2 directly.
- `go-sdk-v1-0-1` is archived first, so the enable-flags requirement is MODIFIED here to allow `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`.
- The gRPC requirement is replaced by a renamed one (REMOVED plus ADDED) because `openspec validate` refuses to drop the old scenario title "Reflection is always on" inside MODIFIED.

