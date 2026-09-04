# _service-python — gRPC (grpc.aio) service seed

A **seed template**, not a finished service: the minimal, consistent, runnable-
in-principle skeleton for a Python `grpc.aio` service in the platform. It grows
into **team-ai** (semantic search + chat/RAG over Qdrant), owner of
`platform.search.v1` and `platform.chat.v1`.

## What's in the box

- `grpc.aio` server with health + reflection and one seed RPC
  (`platform.search.v1.SearchService.SearchListings`, returning mock hits).
- Two server interceptors: **auth** (resolves a `Principal`, `require_scopes`)
  and **tracing** (OTel + W3C `traceparent`, bridges `x-request-id`).
- `pydantic-settings` config (flat, `extra="forbid"`), in-memory repository
  behind a `Protocol`, alembic placeholder, uv-based multi-stage Dockerfile.

## Instantiate

1. **Copy** this directory to a new repo and **rename** `team-service`
   throughout (`pyproject.toml` name + `[project.scripts]`, `OTEL_SERVICE_NAME`
   default in `src/config.py` and `.env.example`).
2. **Vendor the proto** as a git submodule and pin a tag — see
   [`proto-vendor/README.md`](proto-vendor/README.md):
   ```bash
   git submodule add <platform-core-git-url> proto-vendor/platform-core
   (cd proto-vendor/platform-core && git checkout proto/v0.1.0)
   ```
3. **Generate stubs** into the git-ignored `generated/`:
   ```bash
   make proto
   ```
4. **Configure + run**:
   ```bash
   cp .env.example .env      # set AUTH_BEARER_TOKEN etc.
   uv sync --extra dev
   make run
   ```
   Probe it with `grpcurl -plaintext localhost:50052 list` (reflection is on).

## Generated proto layout & the `platform` shadowing caveat

buf generates with source-relative paths, so every generated module lands under
`generated/platform/<domain>/v1/` and imports its siblings absolutely
(`from platform.common.v1 import common_pb2`). To make those resolve, `make proto`
plus `src/server.py` put `generated/` on `sys.path`.

**Caveat:** the generated top-level package is literally `platform`, and once
`generated/` is on `sys.path` it **shadows Python's stdlib `platform` module**
for the whole process. Any code — yours or a third-party dependency — that does
`import platform` expecting the stdlib (e.g. `platform.system()`) will instead
resolve to the generated package and break. This happens because the proto
package root is `platform.*` (kept deliberately; see ADR/contract).

**Recommended hardening:** rewrite the generated absolute imports into relative
imports with [protoletariat](https://github.com/protocolbuffers/protoletariat),
so the generated tree can live under a private `generated` package with no
top-level `platform` on `sys.path`. Run it after `buf generate` — the Makefile
ships an optional `proto-relative` target that does exactly this:

```bash
protol --create-package --in-place --python-out generated buf --config-path buf.gen.yaml
# then import as: from generated.platform.search.v1 import search_pb2
# and drop the sys.path insert in src/server.py
```

`protoletariat` is a dev dependency; the default `make proto` keeps the plain
sys.path approach so the seed works with no extra setup.

## Merge gate

```bash
make check     # ruff check + ruff format --check + pyright + pytest
```
`make test` runs just the suite. The seed tests pass **without** generated code
(they cover the pure helpers: `require_scopes`, config).

## Rules (platform conventions)

- **Don't edit `generated/`.** It is a build artifact — regenerate from the
  pinned proto submodule (`make proto`). It is git-ignored on purpose (ADR-0001:
  `platform-core` never writes generated code into consumer repos).
- **Own your data.** This service owns its own storage (Qdrant / Postgres).
  Never reach into another service's database — cross-service data comes over
  gRPC (Rule 3).
- **Auth is resolved, not trusted.** The auth interceptor turns a credential
  into a `Principal`; today that's a static bearer token, later JWT/cookie at
  the edge (ADR-0003). Business code only ever sees a resolved `Principal` and
  gates with `require_scopes(...)`.

## Layout

```
src/
  config.py              pydantic-settings (flat, extra="forbid")
  server.py              grpc.aio assembly; guards generated imports
  main.py                entrypoint (asyncio.run)
  repository.py          Protocol + in-memory stub (Qdrant/Postgres plug here)
  interceptors/
    auth.py              Principal, contextvar, require_scopes
    tracing.py           OTel setup + x-request-id bridge
tests/test_health.py     pure-helper tests (no generated code needed)
proto-vendor/            git submodule with platform-core proto (pinned)
buf.gen.yaml             python + pyi + grpc-python -> ./generated
Makefile                 proto | run | check | test | migrate(optional)
alembic.ini, migrations/ migration placeholder (own DB only)
Dockerfile               multi-stage, uv, non-root uid 1001
```
