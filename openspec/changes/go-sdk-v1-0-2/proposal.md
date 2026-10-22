## Why

Four decisions of 2026-10-22 (user rule: no dependency on/off flags and no runtime fallbacks; importing a dependency means using it) change SDK v1.0.1 before any service adopts it.

- `postgres.OpenOrLocal` lets a service boot without its database when `ENV` is local or test. That is a runtime fallback. Every service uses `postgres.Open`.
- `grpcx.NewServer` always registers gRPC reflection, a debug surface that must stay off in production.
- The gRPC health service has one status. A pod liveness probe on it restarts a service whenever Postgres or Kafka is down, which turns a dependency outage into a restart loop. Liveness must not depend on dependencies; readiness must.
- The chart `platform-gitops/charts/service` can only probe the overall status, so a service cannot opt in to a separate liveness name.

Source: `plans/go-sdk-migration/PLAN.md` (Decisions, "Kept flags"). Builds on `go-sdk-v1-0-1`, which is archived on this branch; its requirements are in `openspec/specs/go-platform-sdk`. The Python team-ai spec `openspec/specs/flag-trim` is unaffected: team-ai keeps its own settings.

initiative: go-sdk-migration

## What Changes

- **REMOVED** requirement "Local-only database fallback"; `postgres.OpenOrLocal`, its tests and README mentions are deleted. **BREAKING** for any caller; none exists on main (0 of 15 services import the SDK).
- **MODIFIED** "Settings load from environment with the services' existing tags": `config.GRPC` gains `ReflectionEnabled` (`GRPC_REFLECTION_ENABLED`, default false) and `ResolveFor(Runtime)`, which `Load` runs before validation and which refuses true when `Runtime.IsStrict()`. The sentence "There is no `GRPC_REFLECTION_ENABLED`" is replaced.
- **REMOVED + ADDED** "gRPC servers share one interceptor chain and safe errors" becomes "gRPC servers share one interceptor chain, safe errors and opt-in reflection": reflection is registered only when `Config.ReflectionEnabled` is true; scenarios "Reflection is off by default", "Reflection is served locally when enabled", "Strict ENV refuses reflection".
- **MODIFIED** "The SDK has no dependency enable flags": allowed flags are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`.
- **MODIFIED** "Health reflects dependency checks and drains on shutdown": `healthx.Health` serves the service name `liveness`, SERVING from `New`, independent of checks, NOT_SERVING only after `Drain()`.
- **ADDED** "The shared service chart can probe liveness on the liveness health service": chart value `probes.livenessService` (default empty) becomes `grpc.service` of the gRPC `livenessProbe` only. Services set `liveness` when they move to the SDK; others render unchanged.
- Release `v1.0.2` in `README.md` and `CHANGELOG.md`; the user cuts tag `platform-core/packages/go-sdk/v1.0.2` after merge.

## Impact

Code: `platform-core/packages/go-sdk/pkg/{postgres,config,grpcx,healthx}`, README, CHANGELOG; `platform-gitops/charts/service`; `platform-e2e` render scenarios; `platform-core/FEATURES.yaml`.

## NFR

- Liveness `Check` is answered from memory: no I/O, p99 under 5 ms, unaffected by a hung dependency check.
- Readiness semantics unchanged: a failing check flips `""` to NOT_SERVING within one interval.
- Boot: an unreachable database makes `postgres.Open` return within its existing 5 s ping bound.
- Chart: services that do not set `probes.livenessService` render byte-identical manifests (no ArgoCD diff, no pod restart).

## Data classification

No personal or sensitive field is read, stored or logged by this change. Health names, reflection flag and probe values are operational configuration. Consent basis and retention: not applicable.

## Threat model

- Assets: the gRPC surface description (service and method names), pod availability.
- Abuse case 1: reflection left on in production discloses every method and message type to anyone who can reach the port. Control: default off, and `Load` refuses `GRPC_REFLECTION_ENABLED=true` when `ENV` is staging or prod (strict spellings per `Runtime.IsStrict`) before anything else runs.
- Abuse case 2: a dependency outage restarts every pod at once (liveness tied to readiness), amplifying the outage. Control: separate `liveness` status plus the chart opt-in.
- Abuse case 3: a service boots with a nil pool in a local-style `ENV` mislabelled in a deployment. Control: the fallback no longer exists; unreachable database fails boot in every `ENV`.
- Authz rule: the health service and `liveness` name carry no data and need no principal (as today); reflection, when enabled, is local or test only.
