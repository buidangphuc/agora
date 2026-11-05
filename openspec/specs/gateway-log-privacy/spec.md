# gateway-log-privacy Specification

## Purpose
TBD - created by archiving change gateway-principal-log-fingerprint. Update Purpose after archive.

## Requirements

### Requirement: Gateway request logs carry a keyed principal fingerprint, never the raw id
The gateway SHALL log `principal_fp` and `principal_kind` on every `edge.request` line and SHALL NOT log a
`principal` key or the raw principal id anywhere in gateway logs.

#### Scenario: GW-FP-1 gRPC request by a user logs fingerprint and kind
- **WHEN** a user principal with id `user-1` completes a unary gRPC call through the edge with secret `s3cret`
- **THEN** the captured `edge.request` JSON line SHALL have `principal_kind="user"` and `principal_fp` equal to
  `kafkax.Fingerprinter("s3cret").Of([]byte("user-1"))` (12 lowercase hex), and no key `principal`, and the string
  `user-1` SHALL NOT appear anywhere in the captured output
- **VERIFIED BY**: `team-gateway/internal/edge` `TestGWFP1_GRPCUserLogsFingerprintAndKind`

#### Scenario: GW-FP-2 HTTP request by a user logs fingerprint and kind
- **WHEN** a user with a valid bearer token for id `user-1` calls an HTTP edge route (the wrapper in `http.go`)
- **THEN** the captured `edge.request` line SHALL have `principal_kind="user"`, the same `principal_fp` as GW-FP-1
  for the same secret, no key `principal`, and no occurrence of `user-1`

#### Scenario: GW-FP-3 service principal is classified
- **WHEN** a request is authenticated as a principal of `platform.common.v1.PrincipalType` `PRINCIPAL_TYPE_SERVICE` with id `svc-order`
- **THEN** the `edge.request` line SHALL have `principal_kind="service"` and `principal_fp` of `[]byte("svc-order")`, and no raw `svc-order`
- **VERIFIED BY**: `team-gateway/internal/edge` `TestGWFP3_ServicePrincipalIsClassified`

#### Scenario: GW-FP-4a anonymous gRPC caller has kind and no fingerprint
- **WHEN** a gRPC request without a token completes through the edge interceptors
- **THEN** the `edge.request` line SHALL have `principal_kind="anonymous"` and neither `principal_fp` nor `principal`
- **VERIFIED BY**: `team-gateway/internal/edge` `TestGWFP4a_AnonymousGRPCHasKindNoFingerprint`

#### Scenario: GW-FP-4b anonymous HTTP caller has kind and no fingerprint
- **WHEN** an HTTP request without a token completes through the `http.go` wrapper
- **THEN** the `edge.request` line SHALL have `principal_kind="anonymous"` and neither `principal_fp` nor `principal`

#### Scenario: GW-FP-5 invalid token is not logged as a user
- **WHEN** the bearer token is invalid and the call is refused UNAUTHENTICATED 
- **THEN** the `edge.request` line SHALL have `principal_kind="anonymous"` and no `principal_fp`

#### Scenario: GW-FP-6a fingerprint is stable
- **WHEN** the same id is logged by two gateway instances configured with the same secret
- **THEN** both `principal_fp` values SHALL be equal
- **VERIFIED BY**: `team-gateway/internal/edge` `TestGWFP6a_FingerprintStableAcrossInstancesSameSecret`

#### Scenario: GW-FP-6b fingerprint depends on the secret
- **WHEN** the same id is logged by an instance with a different secret
- **THEN** its `principal_fp` SHALL differ from that of the first secret
- **VERIFIED BY**: `team-gateway/internal/edge` `TestGWFP6b_FingerprintDiffersForDifferentSecret`

#### Scenario: GW-FP-6c fingerprint distinguishes ids
- **WHEN** two different ids are logged under one secret
- **THEN** their `principal_fp` values SHALL differ

#### Scenario: GW-FP-7 no raw id on error paths
- **WHEN** an upstream call for user `user-1` fails (e.g. UNAVAILABLE) and `logUpstreamError` and `edge.request` both log
- **THEN** no captured gateway log line SHALL contain `user-1`
- **VERIFIED BY**: `team-gateway/internal/edge` `TestGWFP7_UnavailableUpstreamErrorLogsNoRawID`

#### Scenario: GW-FP-8 rate-limited request still fingerprints
- **WHEN** a user's request is refused with RESOURCE_EXHAUSTED (gRPC) or 429 (HTTP)
- **THEN** its `edge.request` line SHALL carry `principal_fp`/`principal_kind="user"` and no raw id
- **VERIFIED BY**: `team-gateway/internal/edge` `TestGWFP8_RateLimitedGRPCStillFingerprints`

### Requirement: Gateway refuses to boot without LOG_KEY_FP_SECRET in a strict environment
The gateway SHALL load `LOG_KEY_FP_SECRET` through `sdkconfig.LogKeyFP` and SHALL fail at boot when it is empty and ENV is staging or production.

#### Scenario: GW-FP-9 empty secret fails boot in a strict ENV
- **WHEN** config is loaded with ENV=staging (or production) and `LOG_KEY_FP_SECRET` unset or empty (error case)
- **THEN** config load SHALL return an error naming `LOG_KEY_FP_SECRET`, so the process exits non-zero before listening
- **VERIFIED BY**: `team-gateway/internal/config` `TestGWFP9_StrictEnvEmptyLogKeySecretFailsBoot`

#### Scenario: GW-FP-10 whitespace-only secret is treated as empty
- **WHEN** config is loaded with ENV=staging and `LOG_KEY_FP_SECRET` set to `"   "` (boundary case)
- **THEN** config load SHALL fail as in GW-FP-9, because `LogKeyFP.ResolveFor` already tests `strings.TrimSpace(Secret) == ""` (verified in `platform-core/packages/go-sdk/pkg/config/groups.go`) and nothing reaches `NewFingerprinter` untrimmed-empty
- **VERIFIED BY**: `team-gateway/internal/config` `TestGWFP10_WhitespaceOnlyLogKeySecretFailsBoot`

#### Scenario: GW-FP-10b local ENV uses the SDK default
- **WHEN** config is loaded with ENV=local and `LOG_KEY_FP_SECRET` unset
- **THEN** the secret SHALL equal the SDK local default `local-dev-log-key-fp-secret`
- **VERIFIED BY**: `team-gateway/internal/config` `TestGWFP10b_LocalEnvUsesSDKDefaultLogKeySecret`

#### Scenario: GW-FP-11 secret is never logged
- **WHEN** the gateway boots with `LOG_KEY_FP_SECRET=s3cret` and handles requests
- **THEN** the string `s3cret` SHALL NOT appear in any captured log line, including the effective-config dump if any

### Requirement: Deployment declares the secret
The secret SHALL be declared where the gateway env is declared, mirroring team-identity and team-domain.

#### Scenario: GW-FP-12a compose and example env declare the key
- **WHEN** a lint reads `docker-compose.services.yaml` (service team-gateway) and `team-gateway/.env.example`
- **THEN** each SHALL set `LOG_KEY_FP_SECRET` to a non-empty local-only value
- **VERIFIED BY**: `scripts/asdlc/tests/test_gateway_log_key_fp_secret_declared.py` `test_GW_FP_12a_compose_declares_key_with_non_empty_local_value`

#### Scenario: GW-FP-12b README documents the key
- **WHEN** a lint reads the `team-gateway/README.md` env table
- **THEN** it SHALL contain a `LOG_KEY_FP_SECRET` row
- **VERIFIED BY**: `scripts/asdlc/tests/test_gateway_log_key_fp_secret_declared.py` `test_GW_FP_12b_readme_env_table_has_key_row`

#### Scenario: GW-FP-12c gitops injects the key from Vault with no literal
- **WHEN** a lint reads `platform-gitops/platform/vault-config/vault-config.yaml`, `platform-gitops/envs/services/team-gateway.yaml` and `platform-gitops/argocd/apps/team-gateway.yaml`
- **THEN** the gateway SHALL have `secrets.enabled: true` with its Vault bag seeded with `LOG_KEY_FP_SECRET`, and none of the three files SHALL hold a literal secret value for the gateway
- **VERIFIED BY**: `scripts/asdlc/tests/test_gateway_log_key_fp_secret_declared.py` `test_GW_FP_12c_vault_seeds_key_into_gateway_bag`

### Requirement: `principal` is a forbidden log key repo-wide
`platform-core/packages/go-sdk/.golangci.yml` SHALL list `principal` under sloglint `forbidden-keys`.

#### Scenario: GW-FP-13a lint rejects the raw key
- **WHEN** golangci-lint with the go-sdk config runs on a file containing `slog.Info("x", "principal", id)`
- **THEN** it SHALL report a sloglint forbidden-key finding
- **VERIFIED BY**: `team-gateway/internal/lintcfg` `TestGWFP13a_LintRejectsRawPrincipalKey`

#### Scenario: GW-FP-13b no other Go repo is flagged
- **WHEN** golangci-lint with the updated config runs over `team-gateway` after this change and over every other Go repo
- **THEN** the number of `principal` forbidden-key findings SHALL be zero
- **VERIFIED BY**: `team-gateway/internal/lintcfg` `TestGWFP13b_NoPrincipalFindingsInGateway`

#### Scenario: GW-FP-14 e2e-observable gateway log check
- **WHEN** the e2e harness makes one authenticated request through the stack, then reads the team-gateway container log via `docker logs` (as `oic_inv_stack.container_logs` does for other services)
- **THEN** the log SHALL contain an `edge.request` line with `principal_fp` matching `^[0-9a-f]{12}$` and `principal_kind="user"` and not the test user's id
