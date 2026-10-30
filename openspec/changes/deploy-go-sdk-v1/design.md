## Context

The service changes each end with a "Deploy track" that is not their own code. This change merges those lists into one coherent set of file edits and adds the pieces no single service owns: a chart-level PreSync migrate Job, raw-manifest overlays, secret wiring, and the shared `deploy-runtime` requirements. It is built last and merges together with the images. Nothing here changes service behaviour; every scenario is checked statically (`helm template`, YAML parse, kustomize build, compose parse).

## Decisions

### D1 Per-service edit matrix

Rules applied to every service: delete removed flags and env vars, keep everything else; add `GRPC_REFLECTION_ENABLED=true` in local compose only; baseline and the argocd inline block carry `ENV: "local"`; staging and prod overlays carry `ENV: "staging"` and `ENV: "production"`; `probes.livenessService: liveness` in baseline and inline block; no staging or prod file ever sets `GRPC_REFLECTION_ENABLED` or `MOCK_DATA`.

| Service | Delete (compose and gitops) | Add or change | Migrate Job |
|---|---|---|---|
| team-chat | `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `OUTBOX_CLAIM_LOCK_SECONDS`, `OUTBOX_MAX_ATTEMPTS`, `POSTGRES_HOST` | `OUTBOX_CLAIM_LOCK=60s`, `OTEL_ENABLED=false`, `GRPC_PORT=50057` (gitops env); staging and prod overlays new | yes, `server migrate up` |
| team-verification | `DATABASE_ENABLED` | `UPSTREAM_AUDIT_ADDR` comment; overlays new; golden regenerated | yes |
| team-search | `DATABASE_ENABLED`, `KAFKA_ENABLED`; gitops `healthPath` NOTE comment | `grpcPort: 50052`; indexer: no database, `ENV`, `LOG_KEY_FP_SECRET` | yes (server image); indexer none |
| team-audit | `DATABASE_ENABLED` | overlays new; `DATABASE_URL` from Vault | yes |
| team-notification | `KAFKA_ENABLED` | overlays new; `LOG_KEY_FP_SECRET` from Vault | yes |
| team-analytics consumer and ga4-relay | `KAFKA_ENABLED`, `GA4_EXPORT_SOURCE`, `GA4_RELAY_PROBE_PORT`, port 8085 | gRPC probes on 50059 with `liveness`; `analytics.events.dlq` and `listing.events.analytics.dlq` topics; strict overlays; compose relay `ENV` | none (no Postgres) |
| team-promotion | `DATABASE_ENABLED`, `FEATURE_FLAGS_ENABLED`, `KAFKA_ENABLED`, `FEATURE_FLAGS_EVAL_TIMEOUT_MS`, `UPSTREAM_CALL_TIMEOUT_SECONDS` | `FEATURE_FLAGS_EVAL_TIMEOUT=500ms`, `KAFKA_BROKERS`, `FLIPT_ADDR`; both upstream addresses in every overlay | yes (no outbox requeue: new table) |
| team-payment | `DATABASE_ENABLED`, `KAFKA_ENABLED`, `PAYMENT_SETTLEMENT_CONSUMER_ENABLED`, `MOCK_PAYMENTS` | `MOCK_DATA=payment-provider` (local compose and local argocd only), `LOG_KEY_FP_SECRET` from Vault, both upstream addresses non-empty | yes |
| team-engagement | `DATABASE_ENABLED`, `KAFKA_ENABLED`, `ENGAGEMENT_OUTBOX_RELAY_INTERVAL` | `OUTBOX_POLL_INTERVAL=500ms`, `LOG_KEY_FP_SECRET`, overlays new | yes (migration 0014 copies legacy outbox rows) |
| team-domain | `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `IDENTITY_CONSUMER_ENABLED` | `KAFKA_BROKERS`, `GRPC_PORT=50051`, `identity.events.dlq` topic, compose `depends_on` redpanda | yes (0016 requeues failed rows) |
| team-gateway | `HTTP_PORT`, `KAFKA_ENABLED`, `healthPath` | `GRPC_PORT=8080`, `grpcPort: 8080`, `ENV=local` in compose | none (no database) |
| team-order | `DATABASE_ENABLED`, `FEATURE_FLAGS_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `OUTBOX_CLAIM_LOCK_SECONDS`, `FEATURE_FLAGS_EVAL_TIMEOUT_MS` | `OUTBOX_CLAIM_LOCK=60s`, `FEATURE_FLAGS_EVAL_TIMEOUT=500ms`, `LOG_KEY_FP_SECRET`, four upstream addresses | yes (0011 to 0022) |
| team-identity | `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `OUTBOX_CLAIM_LOCK_SECONDS` | `OUTBOX_CLAIM_LOCK=60s`, `LOG_KEY_FP_SECRET` | yes (0019 requeues failed rows) |
| team-sharing, team-referral | `DATABASE_ENABLED` removal, `ENV`, overlays and liveness already done by their changes | `migrate up` subcommand in each binary (code track 1.8), compose migrate job on the service image, `migrate.enabled: true` (D9) | yes, `server migrate up` |

Upstream addresses (refusal-requirement table), per service. Every address below is already present in the baseline `envs/services/<svc>.yaml` (and so inherited by the staging and prod layerings) at the time of writing; each row is therefore "already present, verified only", and an edit is made only if the render test finds one empty:

| Service | Addresses | Action |
|---|---|---|
| team-verification | `UPSTREAM_AUDIT_ADDR` | verified only |
| team-identity | `UPSTREAM_AUDIT_ADDR` | verified only |
| team-notification | `UPSTREAM_DOMAIN_ADDR`, `UPSTREAM_IDENTITY_ADDR` | verified only |
| team-domain | `UPSTREAM_AUDIT_ADDR`, `UPSTREAM_AI_ADDR` | verified only |
| team-promotion | `UPSTREAM_AUDIT_ADDR`, `UPSTREAM_DOMAIN_ADDR` | verified only |
| team-order | `UPSTREAM_AUDIT_ADDR`, `UPSTREAM_DOMAIN_ADDR`, `UPSTREAM_PROMOTION_ADDR`, `UPSTREAM_IDENTITY_ADDR` | verified only |
| team-engagement | `UPSTREAM_AUDIT_ADDR`, `UPSTREAM_ORDER_ADDR` | verified only |
| team-payment | `UPSTREAM_AUDIT_ADDR`, `UPSTREAM_ORDER_ADDR` | verified only |
| search indexer (raw manifest) | `UPSTREAM_AI_ADDR` | verified only, as a literal `value` in `indexer.yaml`, inherited by both overlays |

### D2 One ENV rule

The service changes disagree on where `ENV: "local"` lives (chat, promotion, engagement, notification add it to the baseline; verification, gateway, domain, payment add it only to the inline block). One rule is applied: the baseline `envs/services/<svc>.yaml` and the argocd inline block both carry `ENV: "local"` (this is what the merged `sharing` and `referral` changes already do), and the staging and prod overlays override it. A missing overlay would run a deployed cluster with local defaults, so a render test requires an overlay with a strict spelling for every ApplicationSet service. This also removes the need for an allow-list difference in the repo-coherence check between inline block and baseline.

### D3 Chart: PreSync migrate Job and strategy

`charts/service/templates/migrate-job.yaml` renders, only when `migrate.enabled` is true, a `batch/v1` Job named `<name>-migrate` with annotations `argocd.argoproj.io/hook: PreSync` and `argocd.argoproj.io/hook-delete-policy: BeforeHookCreation`, `backoffLimit` and `activeDeadlineSeconds` from values, `restartPolicy: Never`, the same image expression as the Deployment, `args: ["migrate","up"]`, env limited to `ENV` (from `.Values.env.ENV`) plus, when `secrets.enabled` is false, `DATABASE_URL` from `.Values.env.DATABASE_URL`, `envFrom` the service secret (`<name>-env`) only when `secrets.enabled` is true, and the same `serviceAccountName`. It never mounts migrations (embedded in the binary). `values.yaml` adds `migrate: {enabled: false, args: [migrate, up], backoffLimit: 2, activeDeadlineSeconds: 600}` and `strategy: {}`; the Deployment template renders `strategy:` only when set. With the defaults every existing render is byte-identical. When `migrate.enabled` and `secrets.enabled`, the ServiceAccount, SecretStore and ExternalSecret carry `argocd.argoproj.io/hook: PreSync` and `sync-wave: "-1"` with no delete policy, so the secret exists before the Job on a first install. Rollout (canary) is not given a strategy value: only team-gateway may opt in and it has no database.

### D4 Outbox upgrade and rollout order (human, Round 2 item 4)

Per outbox service (chat, payment, engagement, domain, order, identity; promotion has no old relayer): the first rollout scales old pods to 0, runs the PreSync migrate Job (the migration requeues rows parked as `failed` and, for engagement, copies unpublished legacy rows), then starts the new pods. Mechanism, written in `platform-gitops/docs/runbooks/outbox-first-rollout.md`: (1) set the Application sync policy to none, so auto-sync and `selfHeal` cannot revert the manual `kubectl scale`; (2) scale the old Deployment to 0 and wait for no pod; (3) one commit with the image bump and the one-line overlay edit `strategy: {type: Recreate}` (Applications come from an ApplicationSet, so no CLI `--set` exists; the overlay edit is the only mechanism) and one manual sync; (4) re-enable auto-sync and remove the `strategy` line in a follow-up commit. Because the old pods are already at 0 when the PreSync Job runs, Recreate is only a guard against a stray old pod coming back (for example a scale-up by an HPA); it does not order the Job against the old pods. Failure: a failed Job in this sequence leaves the service down, because there is no old pod to keep. The runbook bounds the outage at 30 minutes (assumed target, see tasks 4): fix and re-sync, or scale the old Deployment back on the old image, which is allowed only while the new outbox table has no rows (the old relayer ignores `seq`). Later rollouts: a failed Job fails the sync and the old pods keep running. Rollback after the migration is an image revert only after the new outbox table is drained (see each service's design). Published rows are purged after 7 days by the service itself (SDK `Store.Purge` on a schedule); no manual purge.

### D5 Raw manifests (human, Round 2 item 5)

`platform/search-indexer`, `platform/team-analytics/consumer.yaml` and `ga4-relay.yaml` are not Helm. The base manifests set `ENV: local` for the indexer, the analytics consumer and the relay (`ENV` has no default and an empty one is refused in every ENV, and the local kind Applications point at the base path); the relay and the consumer also set `GRPC_PORT: "50059"` (the relay gains it, because `GRPC_PORT` has no default and the probes target 50059); kustomize overlays `overlays/staging` and `overlays/prod` under each directory patch `ENV` (`staging`, `production`; `GRPC_PORT` is inherited from the base), wire `LOG_KEY_FP_SECRET` (and GA4 keys for analytics) from `secretKeyRef` of the Vault-synced Secret, and set no `MOCK_DATA` or `GRPC_REFLECTION_ENABLED`. `argocd/apps/search-indexer.yaml` and `team-analytics.yaml` keep pointing at the base path for local (kind); the staging and prod Applications for them are generated by the same ApplicationSet extension only when real clusters exist (out of scope, the overlays are the deliverable and are verified by `kustomize build`). Analytics probes: `grpc: {port: 50059}` readiness, `grpc: {port: 50059, service: liveness}` liveness; the relay drops `GA4_RELAY_PROBE_PORT` and port 8085.

### D6 Secrets from Vault

`LOG_KEY_FP_SECRET` is read by chat, notification, payment, engagement, domain, order, identity, search-indexer, the analytics consumer and the relay (promotion and the services without a Kafka consumer do not need it). Services using `secrets.enabled: true` already get the whole `svc/<name>/config` bag through `envFrom`, so no chart change is needed: `vault-config.yaml` seeds a random `LOG_KEY_FP_SECRET` for local into every generic bag when absent (idempotent), and the user provisions staging and prod (human task, listed in tasks 6). The analytics consumer has no ExternalSecret today: a new `platform/team-analytics/externalsecret.yaml` (ServiceAccount, SecretStore, ExternalSecret `team-analytics` reading `team-analytics/config` into Secret `team-analytics-env`) is added and `team-analytics` is appended to `SERVICES` in `vault-config.yaml`. The ga4-relay keeps its `ga4-relay-env` and gains `LOG_KEY_FP_SECRET` in the `ga4-relay/config` bag. Weakness accepted: the strict guard only refuses an empty value; it cannot tell a weak one apart.

### D7 Compose

Every `<svc>-migrate` for a migrating service stops using `<<: *migrate-defaults` (the golang-migrate image) and uses `build` and `image` of the service, `command: ["migrate","up"]`, environment `ENV=local` and the service `DATABASE_URL`, `depends_on: postgres: service_healthy`, `restart: "no"`. The service keeps `depends_on: <svc>-migrate: service_completed_successfully`. `team-sharing-migrate` and `team-referral-migrate` follow the same shape as every other service (D9). `x-migrate` anchor stays while any service still uses it. `redpanda-init` creates `analytics.events.dlq`, `listing.events.analytics.dlq`, `identity.events.dlq`. Local compose never sets `LOG_KEY_FP_SECRET` except `team-engagement` (`local-e2e-log-key-fp-secret`, read by platform-e2e) and `team-order` (`local-dev-key-fp-secret`); the SDK fills a local default for the others.

### D8 Static verification, not cluster verification

Scenarios are verified by: (a) `platform-e2e` `features/ops/gitops_render.feature` with steps in `pear_authz_gitops_steps.py` style (helm template, no stack); (b) `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` (pytest, helm and kustomize, with `yq` where the existing tests use it); (c) `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` (parse of `docker-compose.services.yaml` and `docker compose config -q`). The unit tests that prove the services themselves refuse unsafe settings are cited as the second `VERIFIED BY` of the shared requirement. Whether the PreSync ordering of the ExternalSecret really precedes the Job on a live Argo CD is not provable statically (open issue).

### D9 sharing and referral consistency

Both already: dropped `DATABASE_ENABLED` from compose, `env: {ENV, GRPC_PORT}` in baseline and inline, `probes.livenessService: liveness`, staging and prod overlays. Consistent with D2. Gap closed by Round 3: their binaries get the `migrate up` subcommand in this change (code track 1.8: SDK `pkg/migrate` with the migrations already embedded by `migrations/embed.go`, no boot-time migration), their compose `<svc>-migrate` jobs switch to the service image like every other service (D7), and `migrate.enabled: true` in their baselines and inline blocks, so the chart renders the PreSync Job for them. There is no `migrate.enabled: false` exception left. Their database services are thirteen in total with the other eleven.

### D9b Migrate Job wave

The migrate Job carries no `sync-wave` annotation (default wave 0). The ServiceAccount, SecretStore and ExternalSecret carry wave `-1` under the PreSync hook, so on a first install they are created before the Job.

### D10 Spec text conflicts with decisions, resolved here

- The retired scenario "team-search refuses in-memory storage in production" keeps its title, because `openspec archive` refuses a MODIFIED block that drops a scenario of the current spec, but its text no longer uses `DATABASE_ENABLED` (empty `DATABASE_URL` only); the unreachable-database case is the separate scenario "team-search refuses to boot without a reachable database" (search change 3.0). No scenario mentions `DATABASE_ENABLED`.
- Fallback scenarios for upstream addresses live in unarchived changes: this change archives after all `<svc>-go-sdk-v1` changes; the order is listed in tasks 7.
- The e2e black-box features `boot_guard_search.feature`, `boot_guard_gateway.feature`, `boot_guard_payment.feature` are edited by their service changes' e2e tracks; `boot_guard_order.feature` is new and owned here.

## Clarifications

### Round 1 — 2026-10-23
- Each service change lists its own Deploy track and carries no MODIFIED copy of a shared deploy-runtime requirement; this change collects them. (human, recipe)
- The agora repository is public; the Go module needs no `GOPRIVATE` and no credentials, so no task here sets either. (human)

### Round 2 — 2026-10-23
1. Migrations: every service gets a `migrate up` subcommand in its binary (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `<svc>-migrate` service running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job per service (here). (human)
2. Kafka is not a readiness check in any service (readiness is Postgres and required upstream checks only); the gateway reports consumer state through its gauge. Probes here are therefore `grpc` readiness without service name and liveness with service `liveness`, and nothing probes Kafka. (human)
3. The shared deploy-runtime requirement "Services refuse unsafe configuration in staging and production", and any other shared deploy-runtime requirement, is MODIFIED only by this change. Service changes carry no copy; each service's needed bullet is listed under its Deploy track and folded into the spec delta here. (human)
4. Outbox upgrade: the migration requeues rows parked as `failed`; the first rollout scales old pods to 0 before new pods start (Recreate once, never two relayers); published rows are purged after 7 days by the SDK `Store.Purge` on a schedule in the service. (human)
5. Raw gitops manifests (search-indexer, analytics, ga4-relay) get strict ENV overlays here; `LOG_KEY_FP_SECRET` is provisioned in Vault by the user (human task). (human)
6. Upstream addresses and clients the service uses are required in every ENV with no fallback mode (engagement `UPSTREAM_ORDER_ADDR`, domain `UPSTREAM_AI_ADDR`, order promotion and address clients, identity session store, which is team-identity's own Postgres tables behind `DATABASE_URL` and therefore not a deploy setting; the required addresses are the table in the refusal requirement). Overlays and compose must set them; the fallback scenarios are removed where they live in `openspec/specs` (by the service change) and, where they live only in unarchived changes, those changes archive first (tasks 7). (human)
7. analytics redacts `ga_client_id` from a record before publishing it to the DLQ when `consent.analytics` is false; no deploy edit beyond the new DLQ topics. (human)
8. Local compose sets `GRPC_REFLECTION_ENABLED=true` for every Go service except team-gateway (Clarifications Round 5); staging and prod never do. (human)
9. search keeps `ENABLE_HYBRID_SEARCH` and `ENABLE_RERANKER`; an invalid `TOMBSTONE_TTL` or `TOMBSTONE_PURGE_INTERVAL` refuses boot, so `platform-e2e/compose/search-tombstones.override.yaml` keeps `30s` and `2s`. (human)
10. gateway: `HTTP_PORT` becomes `GRPC_PORT` and `EDGE_REFLECTION_ENABLED` becomes `GRPC_REFLECTION_ENABLED`; no new read circuit breaker; readiness waits until the revocation consumer reached the end offset captured at start, so the gateway Service is not in endpoints before denylist catch-up. (human)
11. analytics: `GA4_API_SECRET`, `GA4_MEASUREMENT_ID` (and `GA4_EXPORT_DATASET` if used) are required in a strict ENV; the relay probe moves to gRPC health; `OTEL_SERVICE_NAME` stays an accepted key. (human)
12. Accepted: order dispute consumer stalls its partition during a team-audit outage; promotion kill-switch becomes live in every cluster (baseline `FEATURE_FLAGS_ENABLED: "false"` deleted); payment DLQ record shape changes; `UPSTREAM_CALL_TIMEOUT*` is removed in favour of the SDK 2 s bound (so compose and gitops delete it). (human)
13. Docker-backed tests must run: a skip is a gate failure (`GO_SDK_REQUIRE_DOCKER=1`); engagement `ListQuestionsByListing` caps page size at 100 (SDK clamp). The static checks of this change need no Docker except `docker compose config`, which fails rather than skips when absent. (human)

### Round 3 — 2026-10-23 (orchestrator, from user decision 1)
1. Every service gets a `migrate up` subcommand, including team-sharing and team-referral. Compose `<svc>-migrate` runs the service image with `migrate up` for all thirteen database services, and Argo CD runs a PreSync migrate Job for all of them. The earlier exception (`migrate.enabled: false` for sharing and referral, golang-migrate compose jobs) is removed from D9. This change adds the code track 1.8 for the two binaries (write-sets `team-sharing/cmd/**`, `team-sharing/internal/app/**`, `team-referral/cmd/**`, `team-referral/internal/app/**` and their tests). (orchestrator)
2. Platform log retention stays "platform default"; no number is set. (orchestrator, human)

### Round 4 — 2026-10-30 (orchestrator, build of task 1.6.1)
- Q1 [layout] 1.6.1: kustomize refuses an overlay nested under a directory whose own `kustomization.yaml` is the base ("cycle detected"). → **The overlays live in sibling directories `platform/search-indexer-overlays/{staging,prod}` and `platform/team-analytics-overlays/{staging,prod}`; `platform/<dir>/overlays` is a relative symlink to them, so `kustomize build platform-gitops/platform/<dir>/overlays/<env>` and the base render at `platform/<dir>` keep their paths and Argo CD reads an in-repo symlink.** (orchestrator decision; moving the base to `base/` would change the Argo CD app paths, push-images.sh and the base render scenarios)

### Round 5 — 2026-10-30 (human, full e2e run)
- Q1 [security] decision 8: with `GRPC_REFLECTION_ENABLED=true` on team-gateway the public edge on 8080 serves reflection, which breaks the pear-edge scenario "reflection is not served by default". → **team-gateway carries no `GRPC_REFLECTION_ENABLED` in local compose; every other Go service keeps it.** (human)
