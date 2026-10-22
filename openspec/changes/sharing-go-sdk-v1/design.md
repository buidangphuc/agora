## Clarifications

### Round 1 — 2026-10-22

- Target SDK is `v1.0.2`, not `v1.0.1` (human). v1.0.2 removes `postgres.OpenOrLocal`, turns gRPC reflection off by default (`GRPC_REFLECTION_ENABLED`, refused when true in a strict ENV) and adds a `liveness` health service that does not depend on the database; the chart gets an opt-in `probes.livenessService`.
- Reflection is off unless `GRPC_REFLECTION_ENABLED=true` in a local ENV; "always on" is dropped (human). team-sharing sets `probes.livenessService: liveness` in its gitops values: NOT_SERVING on database loss affects readiness only, liveness stays SERVING (human).
- SDK behaviour changes are accepted: Canceled and DeadlineExceeded codes, unique violation (23505) to `AlreadyExists`, readiness NOT_SERVING on database loss, panic recovery (human).
- ENV is set through gitops overlays, as for the other services (`envs/staging/services/*`, `envs/prod/services/*`); there is no Vault injection (human). team-sharing gets the same overlays. Ordering: merge and sync the gitops ENV overlays before the image bump; rollback is reverting the image tag (human).
- No DPO sign-off is needed (user rule). Basis for `created_by`, `utm` and the new call logs is legitimate interest, with platform log retention (human).
