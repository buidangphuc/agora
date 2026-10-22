# Changelog

## v1.0.1

- Module path is now `github.com/buidangphuc/agora/platform-core/packages/go-sdk`, fetchable with a `go-sdk/v1.0.1` tag.
- Services require a tagged release; vendoring tooling (`sync-go-sdk.sh`, `check-go-sdk-vendor.sh`) is removed.
- Shared `.golangci.yml` has no service-local block and one `local-prefixes` entry.
