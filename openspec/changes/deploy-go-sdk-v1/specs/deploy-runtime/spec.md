## MODIFIED Requirements

### Requirement: gRPC services are health-probed

The shared `charts/service` chart SHALL, when a service declares a `grpcPort`, render Kubernetes native gRPC `readinessProbe`, `livenessProbe` and `startupProbe` against that port, so a gRPC service that is not yet ready is kept out of endpoints, a hung one is restarted, and a slow-starting one is not killed during boot. Every Go service on the platform (team-chat, team-verification, team-search, team-audit, team-notification, team-promotion, team-payment, team-engagement, team-domain, team-gateway, team-order, team-identity, team-sharing, team-referral, and the team-analytics consumer and ga4-relay) SHALL declare `grpcPort` (or the raw-manifest equivalent) and SHALL set `probes.livenessService: liveness`, so the `livenessProbe` names the health service `liveness`, which does not depend on Postgres or any upstream, while the `readinessProbe` and `startupProbe` carry no service name and follow the overall status (readiness is Postgres and required upstream checks only; no probe checks Kafka). Because Kubernetes permits only one probe of each kind per container, `grpcPort` and `healthPath` are mutually exclusive; team-gateway is probed over gRPC on its `GRPC_PORT` (8080) and only HTTP-only services (team-frontend) use `healthPath`.

#### Scenario: gRPC service renders all three probes

- **WHEN** `helm template` renders `charts/service` for a service with `grpcPort` set (team-domain, port 50051) against its baseline values
- **THEN** the Deployment container has a `readinessProbe`, a `livenessProbe` and a `startupProbe`, each a `grpc:` probe on port 50051, and only the `livenessProbe` has `service: liveness`
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "Every Go service renders a liveness probe by name"

#### Scenario: HTTP service also gets liveness and startup

- **WHEN** `helm template` renders `charts/service` for a service with `healthPath` set (team-frontend)
- **THEN** the container includes an HTTP `readinessProbe` (as today) plus HTTP `livenessProbe` and `startupProbe` on the same path and port
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "An HTTP service ignores the value"

#### Scenario: Every Go service renders a liveness probe by name

- **WHEN** `helm template` renders `charts/service` for each of the fourteen chart-based Go services against its baseline values
- **THEN** each Deployment's `livenessProbe.grpc.service` equals `liveness` and each `readinessProbe.grpc` and `startupProbe.grpc` has no `service` key
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "Every Go service renders a liveness probe by name"; `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_every_go_service_probes_liveness_by_name`

#### Scenario: The gateway renders no HTTP probe

- **WHEN** `helm template` renders `charts/service` for team-gateway against its baseline values
- **THEN** the container has `grpc:` probes on port 8080 and the output contains no `httpGet` probe and no `healthPath`
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "The gateway is probed over gRPC"

#### Scenario: Disabling migrate renders the with-liveness golden

- **WHEN** `helm template team-verification platform-gitops/charts/service --namespace marketplace -f platform-gitops/envs/services/team-verification.yaml --set migrate.enabled=false` runs
- **THEN** the output is byte-identical to the golden render `platform-gitops/tests/golden/team-verification.with-liveness.yaml`
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "The default render matches the with-liveness golden"

### Requirement: Services refuse unsafe configuration in staging and production

When `ENV` is `staging`, `stage`, `prod` or `production` (any case; a strict environment), every Go service SHALL refuse to start with `GRPC_REFLECTION_ENABLED=true` and SHALL report the offending setting by name. In every `ENV` other than `local` and `test`, reflection SHALL be refused, including an unrecognised `ENV`. Per service, in a strict environment:
- team-payment, team-verification, team-gateway and the team-analytics consumer and ga4-relay SHALL refuse a non-empty `MOCK_DATA` before any other validation; local and test environments SHALL accept it.
- team-identity SHALL refuse the committed development signing key (by public-key fingerprint, whatever the `JWT_KID`), `JWT_KID=dev-2026` and `PASSWORD_RESET_EXPOSE_TOKEN=true`, an empty `KAFKA_BROKERS` and an empty `LOG_KEY_FP_SECRET`, reporting every refusal of one start together.
- team-chat, team-notification, team-promotion, team-payment, team-engagement, team-domain and team-order SHALL refuse an empty `KAFKA_BROKERS`; team-chat, team-notification, team-payment, team-engagement, team-domain, team-order, team-search (indexer) and the team-analytics consumer and ga4-relay SHALL refuse an empty `LOG_KEY_FP_SECRET`. team-promotion has no `LOG_KEY_FP_SECRET` requirement.
- team-domain SHALL refuse `STORAGE_ACCESS_KEY` or `STORAGE_SECRET_KEY` equal to `minioadmin`.
- team-analytics SHALL refuse an empty `GA4_API_SECRET`, `GA4_MEASUREMENT_ID` or `GA4_EXPORT_DATASET`.

In every `ENV`, including `local`, every Go service that uses Postgres SHALL refuse an empty `DATABASE_URL` or an unreachable database (no durable-storage switch and no in-memory mode). Every Go service SHALL refuse an empty address for each upstream it calls, per this table (a service not listed for an address does not read it):

| Address | Required by |
|---|---|
| `UPSTREAM_AUDIT_ADDR` | team-verification, team-promotion, team-payment, team-engagement, team-domain, team-order, team-identity |
| `UPSTREAM_ORDER_ADDR` | team-payment, team-engagement |
| `UPSTREAM_DOMAIN_ADDR` | team-order, team-promotion, team-notification |
| `UPSTREAM_PROMOTION_ADDR` | team-order |
| `UPSTREAM_IDENTITY_ADDR` | team-order, team-notification |
| `UPSTREAM_AI_ADDR` | team-domain, team-search indexer |

The team-identity session store is team-identity's own Postgres tables, reached through `DATABASE_URL`; it is not a separate deploy setting and has no address key. `ENV` and `GRPC_PORT` SHALL have no default: an unset or empty value is refused in every `ENV`. Each refusal SHALL exit non-zero with a message naming the setting. In `local` and `test` the strict-only refusals SHALL not apply.

Deployed configuration SHALL never enable the refused settings: no staging or prod overlay, no raw-manifest overlay and no file of the shared chart values SHALL set `GRPC_REFLECTION_ENABLED` or `MOCK_DATA`.

#### Scenario: Reflection is refused in every strict environment

- **WHEN** each Go service's config loads with `ENV` set to `staging`, `stage`, `prod` and `production` in turn and `GRPC_REFLECTION_ENABLED=true`
- **THEN** each load returns an error naming `GRPC_REFLECTION_ENABLED` and the service does not start
- **VERIFIED BY**: `team-chat/internal/config` `TestStrictEnvRefusesReflectionFirst`; `team-verification/internal/config` `TestStrictEnvRefusesReflection`; `team-search/internal/config` `TestLoadRefusesReflectionInStrictEnv`; `team-audit/internal/config` `TestStrictEnvRefusesReflectionBeforeValidation`; `team-notification/internal/config` `TestLoadRefusesReflectionInStrictEnv`; `team-promotion/internal/config` `TestReflectionDefaultOffAndRefusedInStrictEnv`; `team-payment/internal/config` `TestReflectionRefusedInStrictEnv`; `team-engagement/internal/config` `TestStrictEnvRefusesReflection`; `team-domain/internal/config` `TestStrictEnvRefusesUnsafeSettings`; `team-order/internal/config` `TestReflectionRefusedInStrictEnv`; `team-identity/internal/config` `TestStrictEnvRefusesReflectionBeforeValidation`

#### Scenario: An unrecognised environment refuses reflection only

- **WHEN** team-gateway and team-engagement load with `ENV=dev` and `GRPC_REFLECTION_ENABLED=true`, and again with `ENV=dev`, an empty `KAFKA_BROKERS` and reflection unset
- **THEN** the first load is refused naming `GRPC_REFLECTION_ENABLED` and the second load is accepted for the broker check
- **VERIFIED BY**: `team-gateway/internal/config` `TestEnvOutsideBothSetsRefusesOnlyReflection`; `team-engagement/internal/config` `TestUnrecognisedEnvRefusesReflectionButIsNotStrict`

#### Scenario: Local and test environments allow reflection

- **WHEN** team-engagement loads with `ENV=local` and again with `ENV=test`, each with `GRPC_REFLECTION_ENABLED=true`
- **THEN** both loads succeed and reflection is enabled
- **VERIFIED BY**: `team-engagement/internal/config` `TestLocalAndTestAllowReflection`; `team-domain/internal/config` `TestLocalEnvAcceptsDefaultsAndReflection`

#### Scenario: team-payment refuses mock payments in production

- **WHEN** the team-payment image is started with `ENV=production` and `MOCK_DATA=payment-provider`
- **THEN** the process exits non-zero and its log names `MOCK_DATA`
- **VERIFIED BY**: `team-payment/internal/config` `TestMockDataRefusedBeforeValidation`; `platform-e2e` `features/ops/boot_guard_payment.feature` "team-payment refuses mock payments in production"

#### Scenario: team-gateway refuses mock data in staging

- **WHEN** team-gateway loads with `ENV=staging` and a non-empty `MOCK_DATA`
- **THEN** the load is refused naming `MOCK_DATA` before any other validation
- **VERIFIED BY**: `team-gateway/internal/config` `TestStrictEnvRefusesMockDataFirst`; `team-verification/internal/config` `TestStrictEnvRefusesMockDataFirst`; `team-analytics/internal/config` `TestStrictEnvRefusesMockDataBeforeValidation`

#### Scenario: team-identity refuses the development signing key in production

- **WHEN** the team-identity image is started with `ENV=production` and the development signing key from the local compose file
- **THEN** the process exits non-zero and its log names the signing key
- **VERIFIED BY**: `team-identity/internal/config` `TestStrictEnvRefusesDevSigningKey`; `team-identity/internal/app` `TestRunRefusesDevSigningKeyBeforeConnecting`; `platform-e2e` `features/ops/boot_guard_identity.feature` "team-identity refuses the development signing key in production"

#### Scenario: team-identity refuses an exposed reset token in staging

- **WHEN** the team-identity image is started with `ENV=staging`, its own non-development signing key and `PASSWORD_RESET_EXPOSE_TOKEN=true`
- **THEN** the process exits non-zero and its log names `PASSWORD_RESET_EXPOSE_TOKEN`
- **VERIFIED BY**: `team-identity/internal/config` `TestPasswordResetExposeTokenDefaultsOffAndRefusedInProd`; `platform-e2e` `features/ops/boot_guard_identity.feature` "team-identity refuses an exposed reset token in staging"

#### Scenario: Strict environments refuse missing brokers and log key secret

- **WHEN** team-chat, team-notification, team-payment, team-engagement, team-domain and team-order each load with a strict `ENV` and once with an empty `KAFKA_BROKERS` and once with an empty `LOG_KEY_FP_SECRET`
- **THEN** each of the twelve loads is refused naming the empty variable
- **VERIFIED BY**: `team-chat/internal/config` `TestStrictEnvRequiresKafkaBrokers`; `team-notification/internal/config` `TestLoadRequiresKafkaBrokersAndKeyFPSecretInStrictEnv`; `team-payment/internal/config` `TestStrictEnvRequiresBrokersAuditAndKeySecret`; `team-engagement/internal/config` `TestStrictEnvRequiresBrokersAuditAndSecret`; `team-domain/internal/config` `TestStrictEnvRefusesUnsafeSettings`; `team-order/internal/config` `TestStrictEnvRequiresBrokersAndKeySecret`

#### Scenario: team-promotion and team-identity refuse an empty broker list in a strict environment

- **WHEN** team-promotion loads with a strict `ENV` and an empty `KAFKA_BROKERS`, and team-identity loads with a strict `ENV`, an empty `KAFKA_BROKERS` and an empty `LOG_KEY_FP_SECRET`
- **THEN** the team-promotion load is refused naming `KAFKA_BROKERS` and does not mention `LOG_KEY_FP_SECRET`, and the team-identity load is refused naming both variables in one error
- **VERIFIED BY**: `team-promotion/internal/config` `TestStrictEnvRequiresKafkaBrokers`; `team-identity/internal/config` `TestStrictEnvReportsAllRefusals`

#### Scenario: team-domain refuses default storage keys in a strict environment

- **WHEN** team-domain loads with a strict `ENV` and `STORAGE_ACCESS_KEY=minioadmin`, and again with `STORAGE_SECRET_KEY=minioadmin`
- **THEN** each load is refused naming the offending key, and the same values load with `ENV=local`
- **VERIFIED BY**: `team-domain/internal/config` `TestStrictEnvRefusesUnsafeSettings`, `TestLocalEnvAcceptsDefaultsAndReflection`

#### Scenario: Strict environment spellings match in any case

- **WHEN** team-domain loads with `ENV` set to `STAGING`, `Prod` and ` Production ` (surrounding spaces) in turn, each with `GRPC_REFLECTION_ENABLED=true`, and once more with `ENV=Local` and the same flag
- **THEN** the three strict loads are refused naming `GRPC_REFLECTION_ENABLED`, and the `Local` load is accepted
- **VERIFIED BY**: `team-domain/internal/config` `TestStrictEnvRefusesUnsafeSettings`; `team-engagement/internal/config` `TestLocalAndTestAllowReflection`

#### Scenario: ENV and GRPC_PORT have no default

- **WHEN** team-gateway loads with `ENV` unset and team-audit loads with `GRPC_PORT` unset, every other variable valid
- **THEN** the first load is refused naming `ENV` and the second naming `GRPC_PORT`, in `ENV=local` as well as in a strict `ENV` for the second
- **VERIFIED BY**: `team-gateway/internal/config` `TestLoadRequiresEnv`; `team-audit/internal/config` `TestLoadRequiresGRPCPort`

#### Scenario: team-analytics refuses missing GA4 credentials in a strict environment

- **WHEN** the team-analytics consumer loads with a strict `ENV` and an empty `GA4_API_SECRET`, then an empty `GA4_MEASUREMENT_ID`, then an empty `GA4_EXPORT_DATASET`
- **THEN** each load is refused naming the empty variable
- **VERIFIED BY**: `team-analytics/internal/config` `TestStrictEnvRequiresGA4Credentials`, `TestStrictEnvRequiresGA4ExportDataset`, `TestStrictEnvRequiresLogKeyFPSecret`, `TestStrictEnvRequiresKafkaBrokers`

#### Scenario: team-order refuses to start without a database in production

- **WHEN** the team-order image is started with `ENV=production` and no `DATABASE_URL`
- **THEN** the process exits non-zero and its log names `DATABASE_URL`
- **VERIFIED BY**: `team-order/internal/config` `TestLoadRefusesEmptyDatabaseURL`; `platform-e2e` `features/ops/boot_guard_order.feature` "team-order refuses to start without a database in production"

#### Scenario: team-search refuses in-memory storage in production

- **WHEN** the team-search image is started with `ENV=production` and `DATABASE_URL` empty
- **THEN** the process exits non-zero and its log names `DATABASE_URL`, because team-search has no durable-storage switch and no in-memory mode
- **VERIFIED BY**: `team-search/internal/config` `TestServerLoadRequiresDatabaseURL`; `platform-e2e` `features/ops/boot_guard_search.feature` "team-search refuses in-memory storage in production"

#### Scenario: team-search refuses to boot without a reachable database

- **WHEN** the team-search image is started with `ENV=production` and an unreachable `DATABASE_URL`
- **THEN** the process exits non-zero within the 5 s database open bound and its log names the database setting
- **VERIFIED BY**: `team-search/internal/app` `TestRunFailsOnUnreachableDatabase` (subtest `production`); `platform-e2e` `features/ops/boot_guard_search.feature` "team-search refuses to boot without a reachable database in production"

#### Scenario: A required upstream address is refused when empty in every environment

- **WHEN** team-engagement loads with `ENV=local`, and again with `ENV=production`, each with `UPSTREAM_ORDER_ADDR` empty and then with `UPSTREAM_AUDIT_ADDR` empty
- **THEN** all four loads are refused naming the empty address; team-identity and team-verification are covered for `UPSTREAM_AUDIT_ADDR` only
- **VERIFIED BY**: `team-engagement/internal/config` `TestLoadRefusesEmptyOrderAddr`, `TestLoadRefusesEmptyAuditAddr`; `team-verification/internal/app` `TestRunRefusesEmptyAuditAddr`; `team-identity/internal/config` `TestStrictEnvRequiresDependencyAddresses`

#### Scenario: Deployed manifests never enable reflection or mock data

- **WHEN** the staging and prod overlays of every chart-based Go service, the staging and prod raw-manifest overlays and the shared chart values are scanned for `GRPC_REFLECTION_ENABLED` and `MOCK_DATA`
- **THEN** no file sets either key
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_no_deployed_manifest_sets_reflection_or_mock_data`; `platform-e2e` `features/ops/gitops_render.feature` "No deployed overlay enables reflection or mock data"

### Requirement: Deployed overlays declare ENV for every service with a strict guard

The staging and prod overlays in `platform-gitops/envs/` SHALL set `ENV` to `staging` and `production` for every Go service, namely team-chat, team-verification, team-search, team-audit, team-notification, team-promotion, team-payment, team-engagement, team-domain, team-gateway, team-order, team-identity, team-sharing and team-referral. The baseline `envs/services/<svc>.yaml` and the argocd inline block SHALL set `ENV: "local"`. The raw-manifest overlays of search-indexer, team-analytics consumer and ga4-relay SHALL set `ENV` to `staging` and `production`. A missing overlay would silently run the service with local defaults, so the overlay for every service in the ApplicationSet list SHALL exist and SHALL not set `ENV` to `local`.

#### Scenario: The rendered gateway knows its environment

- **WHEN** `charts/service` is rendered for team-gateway against the staging overlay and again against the prod overlay
- **THEN** the gateway container has `ENV` "staging" and "production" respectively
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "The rendered gateway knows its environment"

#### Scenario: Every Go service renders its strict environment

- **WHEN** `charts/service` is rendered for each of the fourteen chart-based Go services against the staging layering and then the prod layering (`envs/<env>/values.yaml`, `envs/services/<svc>.yaml`, `envs/<env>/services.yaml`, `envs/<env>/services/<svc>.yaml`)
- **THEN** every container has `ENV` "staging" in the first render and "production" in the second render
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "Every Go service renders its strict environment"; `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_every_go_service_renders_strict_env`

#### Scenario: A service without an overlay is flagged

- **WHEN** the list of services in `argocd/apps/appset-services.yaml` is compared with the files in `envs/staging/services` and `envs/prod/services`
- **THEN** every Go service in the list has an overlay file in both directories, and the check fails naming any that has none
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_every_appset_go_service_has_both_overlays`

#### Scenario: The argocd inline block mirrors the baseline

- **WHEN** the inline `values:` of `argocd/apps/team-<svc>.yaml` is compared with `envs/services/team-<svc>.yaml` for each Go service
- **THEN** the `env`, `probes`, `grpcPort` and `migrate` keys are equal, except for the closed allow-list of keys that may differ: `env.MOCK_DATA` (team-payment only, value `payment-provider`, inline block only) and `env.LOG_KEY_FP_SECRET` (team-engagement with `local-e2e-log-key-fp-secret` and team-order with `local-dev-key-fp-secret` only, inline block only)
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_inline_mirror_equals_baseline`

## ADDED Requirements

### Requirement: Every database service migrates through a PreSync Job running the service image

For each Go service that owns a Postgres database (team-chat, team-verification, team-search, team-audit, team-notification, team-promotion, team-payment, team-engagement, team-domain, team-order, team-identity, team-sharing, team-referral), `charts/service` SHALL render, when `migrate.enabled` is true, one `batch/v1` Job named `<name>-migrate` with the Argo CD annotations `argocd.argoproj.io/hook: PreSync` and `argocd.argoproj.io/hook-delete-policy: BeforeHookCreation`, the same image as the Deployment, `args: ["migrate","up"]`, `backoffLimit` 2, `activeDeadlineSeconds` 600, `restartPolicy: Never`, no `sync-wave` annotation (default wave 0, later than the `-1` wave of the secret resources), `ENV` from the service values and `DATABASE_URL` from the service's Vault-synced Secret (`envFrom` `<name>-env`) when `secrets.enabled` is true, or from `env.DATABASE_URL` of the values (no `envFrom`) when it is false, and no volume. The Deployment container `args` SHALL never contain `migrate`. When the Job fails (a pod fails more than `backoffLimit` times or runs past `activeDeadlineSeconds`), the sync SHALL fail; for every rollout other than the first rollout of an outbox service the old pods SHALL keep running, and the first rollout of an outbox service follows the failure rule of the rollout requirement below. `migrate.enabled` SHALL default to false, so a chart render without it is byte-identical to the previous one. Services without a database (team-gateway, team-analytics, team-search indexer) SHALL render no migrate Job. The baseline and argocd inline block of every database service SHALL set `migrate.enabled: true`.

#### Scenario: A database service renders one PreSync migrate Job

- **WHEN** `charts/service` is rendered for team-order against its baseline values and the staging overlay
- **THEN** exactly one `Job` named `team-order-migrate` is rendered with hook `PreSync`, delete policy `BeforeHookCreation`, `backoffLimit` 2, `activeDeadlineSeconds` 600, `args` `migrate` and `up`, the Deployment's image, `ENV` "staging" and `envFrom` secret `team-order-env`
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "A database service renders a PreSync migrate job"

#### Scenario: Every database service renders a migrate Job and no other service does

- **WHEN** `charts/service` is rendered for each of the fourteen chart-based Go services and team-frontend against its baseline values
- **THEN** the thirteen database services listed in the requirement (including team-sharing and team-referral) render one migrate Job each, and team-gateway and team-frontend render none
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_migrate_job_only_for_database_services`

#### Scenario: The migrate Job is absent when disabled

- **WHEN** `charts/service` is rendered for team-verification with `migrate.enabled=false`
- **THEN** no `Job` is rendered
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "The migrate Job is absent when disabled"

#### Scenario: The secret exists before the migrate Job runs

- **WHEN** `charts/service` is rendered for team-order with `migrate.enabled=true` and `secrets.enabled=true`
- **THEN** the ServiceAccount, SecretStore and ExternalSecret carry the hook `PreSync` with sync-wave `-1`, and the Job carries the hook `PreSync` and no `sync-wave` annotation (wave 0, later)
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_secret_resources_precede_migrate_job`

#### Scenario: A failing migrate Job is bounded and fails the sync

- **WHEN** `charts/service` is rendered for team-order with `migrate.enabled=true`
- **THEN** the Job has `backoffLimit` 2, `activeDeadlineSeconds` 600, `restartPolicy: Never` and the hook `PreSync`, so a Job that fails three pod attempts or runs past 600 s fails the sync before the Deployment is applied
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_migrate_job_failure_bounds`

#### Scenario: The migrate Job renders without an ExternalSecret

- **WHEN** `charts/service` is rendered for team-order with `migrate.enabled=true`, `secrets.enabled=false` and `--set env.DATABASE_URL=postgres://x/y`
- **THEN** the Job renders with `ENV` and `DATABASE_URL` `postgres://x/y` in `env`, no `envFrom`, and no ServiceAccount, SecretStore or ExternalSecret with a hook annotation is rendered
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_migrate_job_without_secrets`

#### Scenario: The Deployment never migrates at boot

- **WHEN** the Deployment of every database service is rendered against its baseline values with `migrate.enabled=true`
- **THEN** no container `args` or `command` of the Deployment contains `migrate`, and the only container running `migrate up` is the Job
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_deployment_never_runs_migrate`

#### Scenario: The Job takes no migrations from a mounted volume

- **WHEN** the rendered migrate Job of any service is inspected
- **THEN** it is rendered without `volumes` and without `volumeMounts`, so a missing migrations directory cannot fail it
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_migrate_job_has_no_volume`

### Requirement: Compose runs migrations with the service image and carries no removed flag

In `docker-compose.services.yaml`, every `<svc>-migrate` service of a Go service (team-chat, team-verification, team-search, team-audit, team-notification, team-promotion, team-payment, team-engagement, team-domain, team-order, team-identity, team-sharing, team-referral; every binary has `migrate up`) SHALL build and run that service's image with `command: ["migrate","up"]`, `ENV=local`, the service `DATABASE_URL` and `restart: "no"`, and SHALL NOT use the `migrate/migrate` image or mount `./<svc>/migrations`; the service SHALL `depends_on` it with `condition: service_completed_successfully`. Every Go service block, including the team-analytics ga4-relay, SHALL set `GRPC_REFLECTION_ENABLED=true` and `ENV=local`, and the ga4-relay block SHALL also set `GRPC_PORT=50059`. No Go service block SHALL contain any of the removed keys `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `OUTBOX_CLAIM_LOCK_SECONDS`, `FEATURE_FLAGS_ENABLED`, `FEATURE_FLAGS_EVAL_TIMEOUT_MS`, `IDENTITY_CONSUMER_ENABLED`, `PAYMENT_SETTLEMENT_CONSUMER_ENABLED`, `ENGAGEMENT_OUTBOX_RELAY_INTERVAL`, `MOCK_PAYMENTS`, `EDGE_REFLECTION_ENABLED`, `GA4_EXPORT_SOURCE`, `GA4_RELAY_PROBE_PORT`, `UPSTREAM_CALL_TIMEOUT_SECONDS` or `HTTP_PORT` (team-gateway), and the file SHALL parse with `docker compose config -q`.

#### Scenario: A migrate service runs the service image

- **WHEN** `docker-compose.services.yaml` is parsed and the `team-order-migrate` block is read
- **THEN** its image is `team-order:local`, its command is `migrate` and `up`, its environment is `ENV=local` and the `team-order` `DATABASE_URL`, and it has no volume; the same holds for `team-sharing-migrate` (image `team-sharing:local`) and `team-referral-migrate` (image `team-referral:local`), and no `<svc>-migrate` block uses the `migrate/migrate` image
- **VERIFIED BY**: `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` `test_migrate_services_run_service_image`

#### Scenario: A Go service waits for its migration and no removed key remains

- **WHEN** every Go service block of the parsed compose file is inspected
- **THEN** each database service has `depends_on` its migrate service with `service_completed_successfully`, each block has `GRPC_REFLECTION_ENABLED=true` and `ENV=local`, and none has a removed key
- **VERIFIED BY**: `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` `test_go_services_depend_on_migrate_and_carry_no_removed_key`, `test_go_services_enable_reflection_locally`

#### Scenario: The relay block sets its gRPC port and environment

- **WHEN** the `ga4-relay` block of the parsed compose file is read
- **THEN** it sets `GRPC_PORT=50059`, `ENV=local` and `GRPC_REFLECTION_ENABLED=true`, and sets no `GA4_RELAY_PROBE_PORT`
- **VERIFIED BY**: `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` `test_relay_sets_grpc_port_and_env`

#### Scenario: The gateway listens on GRPC_PORT

- **WHEN** the `team-gateway` block is parsed
- **THEN** it sets `GRPC_PORT=8080` and `ENV=local`, publishes `8080:8080` and sets neither `HTTP_PORT` nor `EDGE_REFLECTION_ENABLED`
- **VERIFIED BY**: `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` `test_gateway_uses_grpc_port`

#### Scenario: The new dead-letter topics are created locally

- **WHEN** the `redpanda-init` command of the compose file is read
- **THEN** it creates `analytics.events.dlq`, `listing.events.analytics.dlq` and `identity.events.dlq`
- **VERIFIED BY**: `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` `test_redpanda_init_creates_new_dlq_topics`

#### Scenario: The compose file parses

- **WHEN** `docker compose -f docker-compose.yaml -f docker-compose.services.yaml config -q` runs
- **THEN** it exits 0, and the command fails the test rather than skipping when the `docker` binary is absent
- **VERIFIED BY**: `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` `test_compose_config_parses`

### Requirement: Gitops baselines carry no removed flag and every Go service sets probes and ports

Each `envs/services/<svc>.yaml` and each argocd inline block of a Go service SHALL carry none of the removed keys listed for compose, SHALL set `ENV: "local"`, SHALL set `probes.livenessService: liveness`, and SHALL make `GRPC_PORT` available to the container (from the image, or from `env.GRPC_PORT` where the image does not set it: team-chat, team-domain, team-gateway). team-promotion SHALL set `KAFKA_BROKERS`, `FLIPT_ADDR` and both upstream addresses and SHALL NOT set `FEATURE_FLAGS_ENABLED`. team-order, team-engagement, team-chat and team-identity SHALL use the unit-suffix-free durations (`OUTBOX_CLAIM_LOCK`, `OUTBOX_POLL_INTERVAL`). `UPSTREAM_CALL_TIMEOUT_SECONDS` SHALL appear nowhere.

#### Scenario: No baseline or inline block carries a removed key

- **WHEN** every `envs/services/*.yaml`, `argocd/apps/team-*.yaml` and raw manifest under `platform/` is scanned for the removed keys
- **THEN** no file contains one of them
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_no_removed_key_in_gitops`

#### Scenario: The promotion kill-switch is live in every cluster

- **WHEN** `charts/service` is rendered for team-promotion against the baseline, staging and prod layerings
- **THEN** no render contains `FEATURE_FLAGS_ENABLED`, each contains `FLIPT_ADDR` and `KAFKA_BROKERS`, and each non-local render contains both `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_AUDIT_ADDR`
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_promotion_kill_switch_live_and_upstreams_set`

#### Scenario: A required upstream address is set in every rendered environment

- **WHEN** `charts/service` is rendered for team-engagement, team-order, team-payment, team-domain, team-promotion, team-verification, team-identity and team-notification against the baseline, staging and prod layerings, and `kustomize build` renders the search-indexer base, `overlays/staging` and `overlays/prod`
- **THEN** every required upstream address of the service in the refusal-requirement table is non-empty in all three renders (`UPSTREAM_AI_ADDR` for the indexer, with the container reading it as a literal `value`)
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_required_upstream_addresses_set_in_every_env`

#### Scenario: The domain service gets its container port

- **WHEN** `charts/service` is rendered for team-domain and team-gateway against their baselines
- **THEN** the container env has `GRPC_PORT` "50051" and "8080" respectively
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_grpc_port_in_container_env`

### Requirement: Raw manifests have strict environment overlays and gRPC probes

The raw manifests `platform/search-indexer/indexer.yaml`, `platform/team-analytics/consumer.yaml` and `platform/team-analytics/ga4-relay.yaml` SHALL have kustomize overlays `overlays/staging` and `overlays/prod` under their directory that set `ENV` to `staging` and `production`, read `LOG_KEY_FP_SECRET` (and, for team-analytics, `GA4_API_SECRET`, `GA4_MEASUREMENT_ID` and `GA4_EXPORT_DATASET`) from a Vault-synced Secret, and set neither `MOCK_DATA` nor `GRPC_REFLECTION_ENABLED`. The team-analytics consumer and relay SHALL use `grpc` readiness on port 50059 and `grpc` liveness on port 50059 with service `liveness`, the relay SHALL expose no port 8085 and no `httpGet` probe, and neither manifest SHALL set `KAFKA_ENABLED` or `GA4_EXPORT_SOURCE`. The team-analytics consumer and ga4-relay SHALL set `GRPC_PORT` to `50059`. The base manifests SHALL set `ENV` `local` for the team-analytics consumer, the team-analytics ga4-relay and the search indexer, because `ENV` has no default and the local (kind) Applications point at the base path; the overlays patch it to `staging` and `production`.

#### Scenario: The analytics consumer overlay renders its strict environment

- **WHEN** `kustomize build platform-gitops/platform/team-analytics/overlays/prod` runs
- **THEN** the consumer container has `ENV` "production", `GRPC_PORT` "50059", `LOG_KEY_FP_SECRET`, `GA4_API_SECRET`, `GA4_MEASUREMENT_ID` and `GA4_EXPORT_DATASET` from `secretKeyRef`, and no `MOCK_DATA` or `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_analytics_overlays_render_strict_env`

#### Scenario: The raw manifest bases boot locally with ENV and GRPC_PORT

- **WHEN** `kustomize build` renders the base of `platform/team-analytics` and of `platform/search-indexer`
- **THEN** the analytics consumer and the ga4-relay have `ENV` "local" and `GRPC_PORT` "50059", and the indexer has `ENV` "local"
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_raw_manifest_bases_set_local_env_and_grpc_port`

#### Scenario: The relay is probed over gRPC

- **WHEN** `ga4-relay.yaml` is parsed
- **THEN** the relay container sets `GRPC_PORT` "50059", has gRPC readiness and liveness probes on 50059 with only liveness naming `liveness`, a `grpc` container port, and no `httpGet` probe, no port 8085 and no `GA4_RELAY_PROBE_PORT`
- **VERIFIED BY**: `platform-gitops/tests/test_ga4_relay_manifests.py` `test_relay_probes_are_grpc`; `platform-e2e/scripts/tests/test_gitops_ga4.py`

#### Scenario: The search indexer overlay renders its strict environment

- **WHEN** `kustomize build platform-gitops/platform/search-indexer/overlays/staging` runs
- **THEN** the indexer container has `ENV` "staging" and `LOG_KEY_FP_SECRET` from the Secret `team-search-env`, and no `KAFKA_ENABLED`
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_indexer_overlays_render_strict_env`

#### Scenario: An overlay that would set reflection is rejected

- **WHEN** the rendered raw-manifest overlays for staging and prod are scanned for `GRPC_REFLECTION_ENABLED` and `MOCK_DATA`
- **THEN** none sets either key
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_no_deployed_manifest_sets_reflection_or_mock_data`

### Requirement: New secrets come from Vault, never from git

`LOG_KEY_FP_SECRET` SHALL reach every Kafka-consuming or event-relaying Go service from its Vault bag `svc/<name>/config` through the existing `<name>-env` Secret (`envFrom`), the team-analytics consumer SHALL read it through a new ExternalSecret of its own, and `platform/vault-config` SHALL seed a random local value when absent. No file in git SHALL contain a staging or prod value of `LOG_KEY_FP_SECRET`, `GA4_API_SECRET` or `DATABASE_URL`. Provisioning the staging and prod values is a human task that SHALL precede the first sync of the new images.

#### Scenario: The analytics consumer reads its secret through an ExternalSecret

- **WHEN** `platform/team-analytics` is built with kustomize
- **THEN** an `ExternalSecret` `team-analytics` reading `team-analytics/config` into Secret `team-analytics-env` is present and the consumer container takes `LOG_KEY_FP_SECRET` from it
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_analytics_consumer_has_external_secret`

#### Scenario: No literal secret is committed for a deployed environment

- **WHEN** the staging and prod overlays and raw-manifest overlays are scanned for a non-empty literal value of `LOG_KEY_FP_SECRET`, `GA4_API_SECRET` or `DATABASE_URL`
- **THEN** none is found
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_no_literal_secret_in_deployed_overlays`

#### Scenario: The Vault seed covers the new key and the analytics service

- **WHEN** `platform/vault-config/vault-config.yaml` is read
- **THEN** `SERVICES` contains `team-analytics` and the generic seed writes a `LOG_KEY_FP_SECRET` only when none exists
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_vault_seed_has_log_key_secret`

### Requirement: The first rollout of an outbox service never runs two relayers

The chart SHALL accept an opt-in `strategy` value that renders the Deployment `spec.strategy` verbatim and renders nothing when the value is empty. The rollout runbook `platform-gitops/docs/runbooks/outbox-first-rollout.md` SHALL order the first rollout of team-chat, team-payment, team-engagement, team-domain, team-order and team-identity as: (1) disable Argo CD auto-sync and `selfHeal` for the Application (`argocd app set <app> --sync-policy none`) so that it cannot revert the scale-down; (2) scale the old Deployment to 0 with `kubectl scale` and wait until no pod of it runs; (3) commit the image bump together with the one-line overlay edit `strategy: {type: Recreate}` and sync once, which runs the PreSync migrate Job (it requeues rows parked as `failed`) and then starts the new pods; (4) re-enable auto-sync and `selfHeal` and remove the `strategy` line in a follow-up commit. Published outbox rows SHALL be purged after 7 days by the service, with no deploy step.

In this first rollout the old pods are already at 0 when the Job runs, so a failed Job leaves the service down. The runbook SHALL state that outcome and its bound: the on-call operator restores service within 30 minutes of the failed sync, by fixing the cause and re-syncing, or by scaling the old Deployment back up on the old image (allowed only while the new outbox table holds no rows, because the old relayer ignores `seq`); the runbook SHALL name the check for that (`SELECT count(*)` of the outbox table is 0). The 30 minute bound is an operator action; it is verified by the document content only. Every other rollout keeps the old pods running when the Job fails.

#### Scenario: The strategy value renders a Recreate Deployment

- **WHEN** `helm template` renders `charts/service` for team-order with the value `strategy.type=Recreate` (passed with `--set` in the render test only; the runbook sets it in the overlay)
- **THEN** the Deployment has `spec.strategy.type` equal to `Recreate`
- **VERIFIED BY**: `platform-e2e` `features/ops/gitops_render.feature` "The strategy value renders a Recreate Deployment"

#### Scenario: Recreate is never committed to a deployed overlay

- **WHEN** the staging and prod overlays and baselines of all services are scanned for `strategy`
- **THEN** none sets it, so every later rollout uses the default rolling update
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_no_committed_recreate_strategy`

#### Scenario: The migrate Job precedes the new pods

- **WHEN** the rendered resources of team-order are ordered by Argo CD phase
- **THEN** the migrate Job is in phase `PreSync` and the Deployment is a normal sync resource, so the Deployment is applied only after the Job succeeds
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_migrate_job_is_presync_deployment_is_not`

#### Scenario: A failed migrate Job in the first rollout is recoverable within the bound

- **WHEN** `platform-gitops/docs/runbooks/outbox-first-rollout.md` is read
- **THEN** it contains, in order, the steps to disable auto-sync and `selfHeal`, to scale the old Deployment to 0, to sync once with the Recreate overlay line, and to re-enable auto-sync; and it contains a failure section naming the outage, the 30 minute restore target, the two restore options and the empty-outbox-table check before scaling the old image back up
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_outbox_runbook_covers_autosync_and_failed_job`

### Requirement: Dead-letter topics are provisioned with explicit retention

`platform-gitops/platform/infra/redpanda.yaml` SHALL create `analytics.events.dlq`, `listing.events.analytics.dlq` and `identity.events.dlq` in staging and prod with `retention.ms=604800000` (7 days), applied on create and by an alter so that an existing topic converges. The local compose `redpanda-init` creates the same three topics with the broker default retention; no retention is set locally because the local stack is disposable.

#### Scenario: The dead-letter topics have a seven-day retention in staging and prod

- **WHEN** `platform-gitops/platform/infra/redpanda.yaml` is read
- **THEN** `analytics.events.dlq`, `listing.events.analytics.dlq` and `identity.events.dlq` are each created with `retention.ms=604800000` and altered to the same value
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_dlq_topics_have_seven_day_retention`

#### Scenario: A dead-letter topic without retention is rejected

- **WHEN** every topic in the dead-letter loop of `platform-gitops/platform/infra/redpanda.yaml` is checked for a `retention.ms` setting
- **THEN** the check fails naming any of the three topics that has none, and passes when all three carry `604800000`
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_dlq_topics_have_seven_day_retention`

### Requirement: team-sharing and team-referral migrate through a migrate up subcommand

The `team-sharing` and `team-referral` binaries SHALL each accept the subcommand `migrate up`, which applies the migrations embedded in the binary (SDK `pkg/migrate`) to the database named by `DATABASE_URL`, then exits 0 without starting a server, and exits non-zero with a message naming `DATABASE_URL` or the failing migration when the database is unreachable or a migration fails. Running the server SHALL NOT apply migrations at boot. Their compose `<svc>-migrate` services and their PreSync Jobs SHALL use the same shape as every other database service (see the two requirements above), so no Go service keeps a golang-migrate image job or a mounted `migrations/` directory.

#### Scenario: team-sharing migrate up applies the embedded migrations

- **WHEN** `team-sharing migrate up` runs against an empty Postgres database
- **THEN** the process exits 0, the `share_links` table exists with the `created_by` column from migration 002, and running the command a second time exits 0 and changes nothing
- **VERIFIED BY**: `team-sharing/internal/app` `TestMigrateUpAppliesEmbeddedMigrations`, `TestMigrateUpIsIdempotent`

#### Scenario: team-referral migrate up applies the embedded migrations

- **WHEN** `team-referral migrate up` runs against an empty Postgres database
- **THEN** the process exits 0, the referrals table from migration 001 exists, and a second run exits 0 and changes nothing
- **VERIFIED BY**: `team-referral/internal/app` `TestMigrateUpAppliesEmbeddedMigrations`, `TestMigrateUpIsIdempotent`

#### Scenario: migrate up fails on an unreachable database or an empty DATABASE_URL

- **WHEN** `team-sharing migrate up` and `team-referral migrate up` each run with an empty `DATABASE_URL`, and again with a `DATABASE_URL` naming a database that does not accept a connection within 5 s
- **THEN** all four runs exit non-zero within 10 s and name `DATABASE_URL` in the message
- **VERIFIED BY**: `team-sharing/internal/app` `TestMigrateUpRefusesEmptyDatabaseURL`, `TestMigrateUpFailsOnUnreachableDatabase`; `team-referral/internal/app` `TestMigrateUpRefusesEmptyDatabaseURL`, `TestMigrateUpFailsOnUnreachableDatabase`

#### Scenario: Server boot never migrates

- **WHEN** the team-sharing and team-referral servers start against an empty Postgres database with no prior `migrate up`
- **THEN** neither start creates any table, so the database schema stays empty until `migrate up` runs
- **VERIFIED BY**: `team-sharing/internal/app` `TestServerBootDoesNotMigrate`; `team-referral/internal/app` `TestServerBootDoesNotMigrate`

#### Scenario: The migrate Job and compose job run the service image

- **WHEN** `charts/service` is rendered for team-sharing and team-referral against their baselines, and `docker-compose.services.yaml` is parsed
- **THEN** each renders one PreSync Job with `args` `migrate` and `up`, `migrate.enabled` is `true` in the baseline and the inline block, and `team-sharing-migrate` and `team-referral-migrate` use the service image with `command: ["migrate","up"]` and no volume
- **VERIFIED BY**: `platform-gitops/tests/test_deploy_go_sdk_v1_manifests.py` `test_migrate_job_only_for_database_services`; `scripts/asdlc/tests/test_compose_deploy_go_sdk_v1.py` `test_migrate_services_run_service_image`
