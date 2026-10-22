Code track only: every scenario gets a `not-testable` entry in `platform-core/FEATURES.yaml` (orchestrator, `## 3`).

## 1. SDK and sync (wave 1)

Owns: `packages/go-sdk/pkg/auth/**`, `packages/go-sdk/.golangci.yml`, `tools/sync-go-sdk.sh`, `tools/sync-go-sdk_test.sh`, `templates/go-service/Dockerfile`, `packages/go-sdk/README.md` (auth and lint lines). All paths are under `platform-core/`.

- [x] 1.1 Red tests first: the two auth tests, then implement `RequireScopesAllowAnonymous`.
- [x] 1.2 Red test first: extend `sync-go-sdk_test.sh` for the service-local block, then implement. Add the empty block to the SDK `.golangci.yml`. Remove the template Dockerfile's `GRPC_PORT` default.

## 2. Template and drift gate (wave 1)

Owns: `platform-core/templates/_service-go/**`, `platform-core/tools/check-go-sdk-vendor.sh`, `platform-core/tools/check-go-sdk-vendor_test.sh`, `platform-core/tools/template_smoke_test.sh`, `.github/workflows/asdlc-gates.yml` (`repo` job only), `scripts/asdlc/ci_local.sh` (`repo` job only).

- [x] 2.1 Write `check-go-sdk-vendor_test.sh` red first (temp clone + tag + vendored copy + one edited file), then the script. Wire it into the CI `repo` job (`git fetch --tags`) and `ci_local.sh`.
- [x] 2.2 Write `template_smoke_test.sh` red first, then rebuild `templates/_service-go` from `examples/hello`, with placeholders `__SERVICE__` and `__PORT__` and a README with the steps.

## 3. Close (orchestrator)

- [x] 3.1 Add the FEATURES entries, run `ci_local`, and tag `go-sdk/v0.2.2` locally.
