# _service-go

Template for a new Go gRPC service (ADR-0016). It is a copy of
`packages/go-sdk/examples/hello` with two placeholders:

- `__SERVICE__`: the service name, also the module path suffix
  (`github.com/buidangphuc/__SERVICE__`), e.g. `team-listing`.
- `__PORT__`: the gRPC port, e.g. `50070`.

It ships no `third_party/` (the sync script creates it) and no `generated/`
(stubs are generated, never committed).

## Steps

1. Copy: `cp -R platform-core/templates/_service-go <repo>/<service>` and copy
   `platform-core/templates/go-service/Dockerfile` next to the Makefile.
2. Rename: replace `__SERVICE__` and `__PORT__` in every file (go.mod, imports,
   Makefile, `.env.example`, `internal/app/app.go`), e.g.
   `grep -rIl -e __SERVICE__ -e __PORT__ . | xargs sed -i '' -e 's/__SERVICE__/team-listing/g' -e 's/__PORT__/50070/g'`
   (drop the `''` after `-i` on Linux).
3. Add your proto under `proto/` with a `buf.gen.yaml` whose `go_package_prefix`
   is `github.com/buidangphuc/<service>/generated`, then `buf generate`. Replace
   the hand-built `ServiceDesc` in `internal/rpc` with your generated
   registration; handlers stay thin (`auth.Require*`, domain call,
   `grpcx.Status`).
4. Vendor the SDK: `platform-core/tools/sync-go-sdk.sh <tag> <service-dir>`
   (for example `go-sdk/v0.2.2`). It writes `third_party/go-sdk`, `make/go-service.mk`,
   `.golangci.yml`, the go.mod replace and tool directive, and runs `go mod tidy`.
5. Verify: `make check` (vet, lint, tests, `.env.example` in sync).

The SDK is not edited in the service. To change it, change `packages/go-sdk`,
tag it and sync again; `tools/check-go-sdk-vendor.sh` fails CI when a vendored
copy differs from its tag.
