## Why

Fifteen Go services move onto go-sdk v1.0.2 (changes `<svc>-go-sdk-v1`, initiative `go-sdk-migration`). Their new images refuse to boot, or run unsafely, against the old compose and gitops files: `ENV` is required with no default, `GRPC_PORT` is required, dozens of `*_ENABLED` flags and unit-suffixed durations are gone, the readiness and liveness health services are split (`probes.livenessService: liveness`), and no service migrates its database at boot any more. Every service change lists the compose and gitops edits it needs under its "Deploy track" and leaves the shared `deploy-runtime` requirements to this change (human decision, Round 2 item 3). This change is the single place that applies those edits, owns the migrate Job and the strict-environment guarantees, and is built last, in the same merge and rollout as the images.

Source of the requirements: the "Deploy track" section of each `openspec/changes/<svc>-go-sdk-v1/tasks.md` (chat, verification, search, audit, notification, analytics, promotion, payment, engagement, domain, gateway, order, identity), the already-applied deploy edits of `sharing-go-sdk-v1` and `referral-go-sdk-v1` (consistency only), `plans/go-sdk-migration/PLAN.md`, and `openspec/specs/deploy-runtime`.

initiative: go-sdk-migration

## What Changes

- **Compose** (`docker-compose.services.yaml`): per service, delete the removed flags and env vars, add `ENV` where missing, add `GRPC_REFLECTION_ENABLED=true` to every Go service (local only), and turn every `<svc>-migrate` service into a job that runs the service image with `migrate up` (no `migrate/migrate` image, no `./<svc>/migrations` mount). `redpanda-init` creates the new dead-letter topics.
- **team-sharing and team-referral binaries** (code, per user decision 1): add the `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations), so their compose migrate jobs run the service image and their PreSync Job is enabled like every other database service.
- **Gitops baseline, argocd inline mirror, staging and prod overlays**: per service, the same deletions and additions, `probes.livenessService: liveness`, an `ENV` in the baseline (`local`) and in new or existing overlays (`staging`, `production`), and `GRPC_PORT` where the image does not supply it to the chart.
- **Shared chart** (`platform-gitops/charts/service`): a new opt-in `migrate` value renders one Argo CD PreSync Job per service that runs the service image with `migrate up`, plus an opt-in `strategy` value for the one-time Recreate rollout. Defaults render byte-identically to today.
- **Raw manifests** (`platform/search-indexer`, `platform/team-analytics` consumer and ga4-relay): strict-ENV kustomize overlays for staging and production, gRPC probes with the `liveness` service for analytics, and secrets wired from Vault through ExternalSecrets.
- **Vault wiring** (`platform/vault-config`): `LOG_KEY_FP_SECRET` is seeded for local and is a human task in staging and prod; new ExternalSecret for the team-analytics consumer.
- **Shared requirements (MODIFIED here only)**: "Services refuse unsafe configuration in staging and production", "Deployed overlays declare ENV for every service with a strict guard" and "gRPC services are health-probed" now cover every service and drop the retired `MOCK_PAYMENTS`, `EDGE_REFLECTION_ENABLED`, `DATABASE_ENABLED` and in-memory text. New requirements cover the PreSync migrate Job, compose migrate jobs, the raw-manifest overlays, secrets wiring, and the rollout order.
- **Static verification**: helm render checks (`platform-e2e` `features/ops/gitops_render.feature`, `pear_authz_gitops_steps.py` style), pytest manifest checks in `platform-gitops/tests`, and compose parse tests in `scripts/asdlc/tests`.
- **BREAKING (deploy)**: promotion kill-switch becomes live in every cluster; the gateway container listens on `GRPC_PORT` and probes by gRPC; first rollout of every outbox service is a one-time Recreate. Details in design.md.

## Non-goals

- Any service code, proto, or SDK change, except the `migrate up` subcommand of team-sharing and team-referral; any e2e run of the whole product (the single full e2e run is a separate step of the initiative).
- Provisioning Vault values in staging and prod, creating real clusters, or rotating secrets (human tasks).
- Adding Kafka topics in staging and prod (human check listed in tasks).
- A canary Rollout for the migrate Job; the chart's canary Rollout opt-in is unchanged.

## Impact

`docker-compose.services.yaml`; `team-sharing/cmd/**`, `team-sharing/internal/app/**`, `team-referral/cmd/**`, `team-referral/internal/app/**`; `platform-gitops/**` (charts/service, envs/**, argocd/apps/**, platform/**, tests/**, FEATURES.yaml); `platform-e2e/tests/e2e/features/ops/**`, step definitions, `platform-e2e/scripts/tests/test_gitops_ga4.py`, `platform-e2e/compose/**`; `scripts/asdlc/tests/**`. Depends on every `<svc>-go-sdk-v1` change (chat, verification, search, audit, notification, analytics, promotion, payment, engagement, domain, gateway, order, identity, sharing, referral) and on `go-sdk-v1-0-2`.

## Non-functional requirements

- Latency: no runtime path changes; the PreSync Job adds one migration run to a sync; the only enforced bound is `activeDeadlineSeconds` 600 (there is no shorter gated target).
- Availability: the first rollout of an outbox service has downtime of one migration run plus one pod start (old pods at 0 until migrate succeeds), accepted once; if its migrate Job fails the service stays down until the runbook restore (fix and re-sync, or scale the old image back up while the new outbox table is empty), bounded at 30 minutes. Argo CD auto-sync and `selfHeal` are disabled for that Application during the window. Later rollouts are rolling with zero downtime; a failed migrate Job fails the sync and leaves the old pods running (PreSync runs before the Deployment changes).
- Limits: migrate Job `backoffLimit` 2 and `activeDeadlineSeconds` 600 by default; `terminationGracePeriodSeconds` stays the Kubernetes default 30 s, above `SHUTDOWN_DRAIN_DELAY + SHUTDOWN_GRACE` (0 s + 10 s).
- Verification budget: static checks only (no cluster); `helm template` per service under 5 s.

## Data classification

This change adds no personal data and moves none. It handles secrets (configuration, not personal data):

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `LOG_KEY_FP_SECRET` (per service, Vault `svc/<name>/config`) | secret, HMAC key for log key fingerprints | keep personal identifiers out of consumer logs and dead-letter logs | not personal data; security control | until rotated; the user provisions it in staging and prod |
| `DATABASE_URL` (Vault, migrate Job env) | secret, credentials | run `migrate up` | not personal data | unchanged |
| `GA4_API_SECRET`, `GA4_MEASUREMENT_ID`, `GA4_EXPORT_DATASET` (Vault `ga4-relay/config`, team-analytics) | secret / config | GA4 relay and export import | existing basis in `ga4-analytics`; unchanged | unchanged |
| Local development secret values in compose and local argocd (`local-e2e-log-key-fp-secret`, `local-dev-key-fp-secret`) | non-secret dev placeholders | local stack only | n/a | never used outside `ENV=local`; the strict-ENV guard refuses an empty value, not a weak one (see design) |

No secret value is written to a file by this change except those local placeholders; staging and prod values are never in git.

## Threat model

- Assets: the strict-ENV guarantee (no staging or prod pod runs with local defaults, reflection or mock data), database schema integrity during migration, outbox ordering (never two relayers), Vault secrets.
- Abuse cases and mitigations:
  - A staging or prod overlay is missing, so the baseline `ENV=local` leaks into a deployed cluster: a render test requires an overlay for every service that sets a strict spelling, and the SDK refuses unsafe settings (reflection, mock data, empty brokers) only when `ENV` is strict, so the overlay is the control; the test fails the build when one is missing.
  - `GRPC_REFLECTION_ENABLED` or `MOCK_DATA` set in a staging or prod overlay or raw manifest: a render test forbids both in every non-local overlay; the services also refuse them at boot.
  - Two outbox relayers run during the first rollout and publish a row twice or out of order: old pods go to 0 before the PreSync Job and the new pods start (one-time Recreate); consumers dedupe on event id.
  - A migrate Job runs with the wrong credentials or a partial secret: it reads only `ENV` and the service's own Vault bag; the Job fails the sync (old pods stay).
  - A pod gets another service's Vault bag: each service keeps its own ServiceAccount, SecretStore and role (`svc/data/<name>/*`); unchanged.
- Authorization rule: no new RPC; the only authorization surface is the Vault policy per service path, unchanged, plus the new `team-analytics` path read only by the team-analytics service account.
