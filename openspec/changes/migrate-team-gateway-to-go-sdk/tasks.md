Code track only: every scenario gets a `not-testable` entry in `team-gateway/FEATURES.yaml`.

## 1. Migrate team-gateway (single packet)

Owns: `team-gateway/**`, the team-gateway block in `docker-compose.services.yaml`, and the team-gateway files in `platform-gitops`, plus the box ticks.

- [x] 1.1 Sync `go-sdk/v0.2.1`. Gateway generation uses `buf generate --template buf.gen.local.yaml`; adapt the Makefile `proto` target and check that the sync script's `buf generate` does the right thing, otherwise report it.
- [x] 1.2 Write the spec tests red first.
- [x] 1.3 Restructure per the proposal, moving the tests. Verify `make check` (Docker).
- [x] 1.4 Remove the always-on flags (if any), write FEATURES and the README, and update compose and gitops. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, `red_first` and `make docker`.

## 2. Use existing SDK packages instead of local copies

Behaviour, wire messages and env names stay the same; existing tests keep their assertions. Where a switch would change a client-visible message or a retry rule, leave that site and note it in the report.

Owns: `team-gateway/**` except `third_party/**`.

- [ ] 2.1 Use the `auth.MD*` metadata key constants instead of redefining the principal keys (`internal/edge/forward.go`). Verify `make check`.
- [ ] 2.2 `callRead` keeps its retry-only-on-Unavailable rule; switch to `resilience` only if that rule is expressible without an SDK change, otherwise leave it. Verify `make check`.

## 3. Adopt go-sdk v1.0.0

Follow `platform-core/packages/go-sdk/README.md` "Upgrading to v1" at tag `go-sdk/v1.0.0`. Behaviour, wire messages and gRPC codes stay the same unless a step below says otherwise; existing test assertions are not loosened. Any earlier unchecked "Use existing SDK packages" items are done here against v1.

Owns: `team-gateway/**`, the team-gateway blocks of `docker-compose*.yaml` and `platform-gitops/**` for team-gateway.

- [ ] 3.1 Sync `go-sdk/v1.0.0` into team-gateway only and fix every breaking change (package renames, ErrorTable, constructors returning errors, ENV required, renamed duration env keys and removed `OUTBOX_MAX_ATTEMPTS` in config, .env.example, compose and gitops; Kafka DLQ header names).
- [ ] 3.2 Replace local copies with SDK packages where the SDK now has them (auth gates, jwtx, idempotency, postgres.OpenOrLocal/WithTx, migrate.Up, grpcx.Status rules keeping today's client text, config IsStrict/PositiveDuration/keepempty, pagination when the encoding is unchanged, lifecycle.Every/HTTP, consume batch consumer and Dedupe.Processed, group-less kafkax client, upstream.NewClient/WithDialOptions, resilience RetryIf, flags.NewOrDisabled, testkit helpers). Sites left local get a one-line reason in the report.
- [ ] 3.3 Verify `make check`, `docker build`, red_first, openspec validate.
