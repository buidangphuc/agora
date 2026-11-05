## Why
The gateway writes the raw principal id under key `principal` in every `edge.request` line (gRPC interceptor
`logRequest` in `team-gateway/internal/edge/interceptors.go`, HTTP wrapper in `internal/edge/http.go`).
A raw user id in logs breaks the repo rule "no raw ids in logs". Operators still need to join the requests of
one caller, so the id is replaced by a keyed fingerprint.

## What changes
- `edge.request` lines (gRPC and HTTP) drop `principal` and carry `principal_fp` (`kafkax.Fingerprinter.Of`,
  HMAC-SHA256, 12 lowercase hex) and `principal_kind` (`anonymous` | `user` | `service`).
- Anonymous callers carry no `principal_fp` (the kind says it all; no id to fingerprint).
- The gateway reuses the SDK env `LOG_KEY_FP_SECRET` via `sdkconfig.LogKeyFP` (same mechanism as team-identity
  and team-domain; user decision, no new env name). `LogKeyFP.ResolveFor` already fills the documented local
  default for a local ENV and fails config load when the value is empty or whitespace-only in a strict ENV
  (staging/prod), so the gateway fails fast at boot there. Declared in `team-gateway/.env.example`,
  `team-gateway/README.md` env table and root `docker-compose.services.yaml` (local value, like team-order/
  team-engagement). In gitops the secret is not a literal: `platform/vault-config/vault-config.yaml` seeds
  `LOG_KEY_FP_SECRET` into `svc/team-gateway/config` and the chart ExternalSecret (`secrets.enabled: true`,
  already set in `envs/services/team-gateway.yaml` and `argocd/apps/team-gateway.yaml`) injects it.
- `principal` added to `forbidden-keys` in `platform-core/packages/go-sdk/.golangci.yml`.
- Survey result: no non-test Go code outside `team-gateway` logs a literal `"principal"` key, so no other
  service fails the new lint rule. The gateway's own two call sites are removed by this change.

initiative: p10-backend-gaps

## NFR
- Latency: one HMAC-SHA256 per request, < 5 microseconds; no measurable change to edge p95.
- Availability: a missing secret in staging/prod refuses boot (fail fast); a local ENV uses the SDK local default; never a fallback to raw ids or to no fingerprint.
- Determinism: same id and secret always give the same `principal_fp`, so joins across replicas hold.
- Limit: fingerprint is 12 hex chars (48 bits); collisions are acceptable for log joining, not for identity.

## Data classification
| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| principal id (user/service id) | personal (pseudonymous id) | resolved in memory for authz and rate limit only; no longer logged | contract/legitimate interest | not persisted by this change; DPO sign-off remains a human item |
| `principal_fp` | pseudonymous, keyed (not reversible without secret) | join one caller's requests in logs | legitimate interest (security/ops) | log retention of the log platform, unchanged |
| `principal_kind` | non-personal | triage by caller type | n/a | same |
| `LOG_KEY_FP_SECRET` | secret | HMAC key | n/a | Vault-managed; rotation changes all fingerprints (history no longer joins across rotation) |

## Threat model
- Assets: user ids, the HMAC secret.
- Abuse cases: (1) log reader recovers user ids; mitigated by HMAC with a secret not present in logs.
  (2) offline brute force of ids by someone holding logs and the secret; secret kept in Vault, never logged.
  (3) a future change reintroduces `principal`; blocked by the sloglint forbidden-key.
  (4) boot with empty secret in staging/prod yields guessable fingerprints; blocked by fail-fast. A local-default secret is acceptable only in local ENV.
- Authz rule: unchanged. Fingerprinting changes logging only; no authorization or rate-limit key uses the fingerprint.
