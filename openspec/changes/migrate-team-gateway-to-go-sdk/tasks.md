Code track only: every scenario gets a `not-testable` entry in `team-gateway/FEATURES.yaml`.

## 1. Migrate team-gateway (single packet)

Owns: `team-gateway/**`, the team-gateway block in `docker-compose.services.yaml`, and the team-gateway files in `platform-gitops`, plus the box ticks.

- [x] 1.1 Sync `go-sdk/v0.2.1`. Gateway generation uses `buf generate --template buf.gen.local.yaml`; adapt the Makefile `proto` target and check that the sync script's `buf generate` does the right thing, otherwise report it.
- [x] 1.2 Write the spec tests red first.
- [x] 1.3 Restructure per the proposal, moving the tests. Verify `make check` (Docker).
- [x] 1.4 Remove the always-on flags (if any), write FEATURES and the README, and update compose and gitops. Verify `features-check`, `spec_sync --strict`, `openspec validate --strict`, `red_first` and `make docker`.
