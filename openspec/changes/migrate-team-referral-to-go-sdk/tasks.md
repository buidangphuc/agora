Code track only: there is no user-facing behaviour change. Every scenario names its test and gets a `not-testable` entry in `team-referral/FEATURES.yaml` (create the file if it is missing).

## 1. Migrate team-referral (single packet)

Owns: `team-referral/**`, the `team-referral` service and migrate blocks in `docker-compose.services.yaml`, and the box ticks in this file.

- [x] 1.1 Run `buf generate`, then `platform-core/tools/sync-go-sdk.sh go-sdk/v0.1.1 team-referral`. Verify with `go build ./...`.
- [x] 1.2 Write the new spec tests first and see them fail.
- [x] 1.3 Restructure per the proposal, copying team-sharing's wiring (`team-sharing/internal/app/app.go`). Move the existing tests with their packages. Verify `make check` (Docker).
- [x] 1.4 Clean the team-referral compose block of removed flags. Write `.env.example`, the README and the FEATURES entries. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, and `make docker` (then remove the image).

## 2. Adopt go-sdk v1.0.0

Follow `platform-core/packages/go-sdk/README.md` "Upgrading to v1" at tag `go-sdk/v1.0.0`. Behaviour, wire messages and gRPC codes stay the same unless a step below says otherwise; existing test assertions are not loosened.

Owns: `team-referral/**`, the team-referral blocks of `docker-compose*.yaml` and `platform-gitops/**` for team-referral.

- [x] 2.1 Sync `go-sdk/v1.0.0` into team-referral only and fix every breaking change (package renames, ErrorTable, constructors returning errors, ENV required, renamed duration env keys in config, .env.example, compose and gitops).
- [x] 2.2 Replace local copies with SDK packages where the SDK now has them: auth type gates and anonymous scope checks, `postgres.OpenOrLocal` (fallback only when local), `postgres.WithTx`, `migrate.Up` in full-apply tests, `grpcx.Status` with rule messages that keep today's client text, `config.Runtime.IsStrict`/`PositiveDuration`/`keepempty`, `pagination` only where the token encoding stays the same, `lifecycle.Every`/`HTTP`, `testkit` helpers (FreePort, Eventually, Recorder, SharedPostgres+Schema, Produce/ReadAll). Sites left local get a one-line reason in the report.
- [x] 2.3 Verify `make check`, `docker build`, red_first, openspec validate.
