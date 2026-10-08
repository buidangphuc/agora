## Context

See proposal.md for the motivation. These are the code facts the approach rests on (agora `feat/ui-system`,
2026-10-08).

**Gateway chain.**
- `team-gateway/internal/edge/interceptors.go` builds the chain request-id → auth → logging → rate limit from
  `connect.UnaryInterceptorFunc`s. Connect skips unary interceptors for streaming handlers.
- `StreamChat` (`edge/chat.go`) therefore resolves the principal inside `Edge.outgoing`. On an invalid token that path
  only drops the scopes, so the call goes ahead as an anonymous principal.

**Plain-HTTP routes.**
- `POST /api/track` (`edge/collector.go`) and `/api/admin/metrics` (`edge/cockpit.go`) are plain `http.Handler`s on the
  mux.
- Each resolves the principal itself; neither is rate limited or logged.

**AI calls.**
- `edge/ai.go` sends the four AI procedures through `callRead`, which uses `EDGE_CALL_TIMEOUT` (5 s) and retries up
  to `RETRY_MAX` on `Unavailable`.

**Reflection.**
- `edge/server.go:104-130` always mounts `grpcreflect` v1 and v1alpha.

**Upstream dial.**
- `internal/upstream/clients.go` calls `grpc.NewClient(addr, insecure, otelStats)` with the default `dns` resolver and
  `pick_first`, so it inherits grpc-go's default reconnect backoff (1 s base, factor 1.6, up to 120 s).
- When a container is recreated with a new IP, the subchannel stays in backoff against the dead address. Re-resolution
  happens only on a transport failure, so calls answer `unavailable` for tens of seconds. The e2e suite measured this
  (it now waits 35 s after a recreate).
- `DIAL_TIMEOUT_SECONDS` is declared and never read.

**Env-example gate.**
- `Makefile:29` runs `go test -run TestEnvExampleInSync`. No such test exists. `CheckEnvExample` is there in
  `internal/config/envcheck.go`.

**Order handler.**
- `team-order/internal/handler/order.go:342` lets the buyer or an admin call `ForceFailSaga`.
- `CreateOrder` (`:53`) accepts any principal.

**Payment handler.**
- `team-payment/internal/handler/wallet_ledger.go` `sellerAccess` binds payouts to the caller's own USER id, but
  checks no scope.

**Identity config.**
- `team-identity/internal/config/config.go` refuses `PASSWORD_RESET_EXPOSE_TOKEN` only when `IsProd()`. The strict-ENV
  set already includes staging, and is used for the signing-key guard.

## Goals / Non-Goals

**Goals:** close the residual requirements with the smallest edge-local changes, keep Rule 2 (the gateway does
auth, rate limit and forwarding only), and keep every new behaviour observable through the edge.

**Non-Goals:** a gateway-side LLM timeout ladder (that is team-ai's job in `ai-path-resilience`), stream duration and
concurrency caps, and service-side reflection.

## Decisions

### D1. One chain, three adapters
- Turn the four unary interceptors into full `connect.Interceptor` implementations: `WrapUnary` plus
  `WrapStreamingHandler`.
- The streaming wrapper runs the same steps:
  - resolve the principal and fail closed on an invalid token;
  - apply the admin policy;
  - take a token from the same limiter;
  - set `X-Request-Id` on the response header before the first message;
  - log once when the handler returns.
- For plain HTTP, add one `edgeHTTP(next, opts)` middleware for `/api/track` and `/api/admin/metrics` that performs
  request-id, the rate limit and the log line. Those handlers already resolve the principal and keep doing it.
- The collector uses a separate limiter (`TRACK_RATE_LIMIT_*`). Beacons are bursty (a page unload flushes a queue),
  and they must not drain the caller's RPC bucket.
- Alternative considered: a generic `net/http` middleware wrapping the Connect handlers too. Rejected because it cannot
  see the Connect procedure for the admin policy, and it would double-log unary calls.

### D2. Stream request cap via Connect's read limit
- Mount the chat handler with `connect.WithReadMaxBytes(STREAM_MAX_REQUEST_BYTES)`. Connect then answers
  `resource_exhausted` before the handler runs.
- Unary handlers keep their current limits.

### D3. AI calls get `callAI`
- `callAI(ctx, fn)` is `callWrite` with the `AI_CALL_TIMEOUT_SECONDS` deadline: one attempt and no retry.
- `callRead`/`callWrite`/`callAI` report the attempt count through a context value that the logging interceptor adds
  to `edge.request` (`attempts`). This is also useful for read retries.

### D4. Fast upstream recovery
- Dial with `grpc.WithConnectParams(grpc.ConnectParams{Backoff: {BaseDelay: 200ms, Multiplier: 1.6, Jitter: 0.2,
  MaxDelay: DIAL_TIMEOUT_SECONDS}, MinConnectTimeout: DIAL_TIMEOUT_SECONDS})`, with a default `DIAL_TIMEOUT_SECONDS`
  of 2.
- Inside a docker or k8s network, a dead IP answers `connection refused` or times out quickly. The `dns` resolver then
  re-resolves on the transport failure, and the short backoff cap means the next attempt reaches the new address.
- No client keepalive is added. The Go upstreams keep grpc-go's default `MinTime` of 5 min, so frequent pings would be
  answered with `GOAWAY too_many_pings`.
- The 5 s spec bound is checked by the destructive e2e. If backoff alone is not enough, the fallback is a `round_robin`
  service config, which re-resolves on every subchannel failure. The integrator records which one was needed in the
  tasks evidence.
- Both options are local to `clients.go`.

### D5. Reflection flag
- `EDGE_REFLECTION_ENABLED` (default false) is checked in `server.go`.
- `config.Validate` returns an error when strict ENV is set and the flag is true. `main` already exits non-zero on a
  `Validate` error with the message, and the boot-guard e2e pattern is the same as `port-security-hardening`.

### D6. Order and payment authorisation
- Edge: add `orderv1connect.OrderServiceForceFailSagaProcedure` to `adminProcedures`. The pinning test in
  `policy_test.go` is updated.
- team-order: `ForceFailSaga` requires the `admin` scope (the owner branch is removed). `CreateOrder` returns
  `PermissionDenied` unless `principal.Type == USER`, checked before the kill-switch and before any reservation.
- team-payment: `RequestPayout`/`RequestWalletPayout` check `slices.Contains(scopes, "listing.write")` after
  `sellerAccess`. Reads are unchanged.

### D7. GitOps and root hygiene
- Add `ENV: staging|production` to `envs/{staging,prod}/services/team-gateway.yaml`.
- team-ai `networkPolicy: {enabled: true, allowFrom: [team-gateway, prometheus]}` goes in both
  `envs/services/team-ai.yaml` and the inline block in `argocd/apps/team-ai.yaml`; the two must stay in sync, as for
  team-payment last wave.
- The vault app comment points at `platform/vault-config`.
- Delete `deploy/vault/bootstrap-vault.sh` and update `deploy/README.md`.
- Pin MinIO in `platform-core/infra/docker-compose.yaml` to the GitOps tag `RELEASE.2024-05-28T17-19-04Z`.
- Port `check_documented_env` from the retired checkout's `scripts/repo_doctor.py` (`ENV_ROW`, `SRC_EXT`,
  `SKIP_DIRS`). README files are excluded from the reference search, and a `--root` argument lets the e2e run it on a
  temp copy.
- Before the gate, fix any real violations the doctor finds: delete the README row or wire the variable. Each fix is
  its own commit.

## Risks / Trade-offs

- [Stream rate limit charges the shared bucket, so a chat-heavy user hits the unary limit sooner] → One stream costs
  one token, the same as a call. The defaults (20 rps, burst 40) are far above human chat use.
- [Per-IP collector limit behind a NAT throttles many visitors as one] → Logged-in visitors are keyed by user. Anonymous
  visitors behind a NAT share a 5 rps / burst 20 bucket. A batch carries up to 100 events, so this is about 2,000 events
  per burst. `TRUSTED_PROXIES` yields the real client IP behind the ingress.
- [The beacon-flood e2e shares the docker-host IP with every other test's browser] → It runs in the destructive serial
  lane.
- [A shorter backoff cap means more dial attempts against a down upstream] → 2 s × 16 upstreams is negligible.
- [Removing buyer access to `ForceFailSaga` breaks scripts that used it as a cancel] → Only e2e uses it. Its scenarios
  switch to the admin in this change.

## Migration Plan

- Deploy order does not matter. There is no proto or data change.
- Rollback is a revert of the gateway, order and payment images.
- Clients that minted SERVICE tokens for `CreateOrder` do not exist.
