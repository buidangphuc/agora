# Changelog

## v1.0.1

- Module path is now `github.com/buidangphuc/agora/platform-core/packages/go-sdk`, fetchable with a `go-sdk/v1.0.1` tag.
- Services require a tagged release; vendoring tooling (`sync-go-sdk.sh`, `check-go-sdk-vendor.sh`) is removed.
- Shared `.golangci.yml` has no service-local block and one `local-prefixes` entry.
- G2: the SDK keeps no enable flag for any dependency.
- G3: new `pkg/mockdata` and config group `MockData` (`MOCK_DATA`); refused when `Runtime.IsStrict()`.
- G4: new `pkg/auditx` with `config.Audit` (`UPSTREAM_AUDIT_ADDR`), `Dial` and a fail-closed `Gate`.
- G5: `consume` and `kafkax` log `key_fp` (HMAC-SHA256 under `LOG_KEY_FP_SECRET`) instead of the raw key; raw keys are scrubbed from error text.
- G6: `grpcx.Status` maps Postgres unique violation `23505` to `AlreadyExists`.
