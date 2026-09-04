# proto-vendor/

This directory is where **platform-core's proto module is vendored** — pinned at
a tag, as a git submodule. It is the input to `buf generate` (see `../buf.gen.yaml`).

Per **ADR-0001** (pin + local-generate), `platform-core` owns the gRPC contracts
under `packages/proto` and versions them with git tags (`proto/vX.Y.Z`). This
service does **not** consume generated code from platform-core; it vendors the
*proto sources* and generates its own Go code locally, into `../generated/`.

## One-time setup: add the submodule

Pin at a released proto tag (placeholder shown — use a real tag that exists):

```bash
# From the repo root. Point the URL at wherever platform-core is hosted/cloned.
git submodule add <platform-core-repo-url> proto-vendor
cd proto-vendor
git checkout proto/v0.1.0     # <- pin at a tag, never a moving branch
cd ..
git add .gitmodules proto-vendor
git commit -m "vendor proto module pinned at proto/v0.1.0"
```

After this, `proto-vendor/` contains the proto module root (the `buf.yaml` plus
`platform/common/`, `platform/listing/`, `platform/search/`, `platform/chat/`),
so imports like `platform/common/v1/common.proto` resolve during codegen.

## Fresh clone

```bash
git submodule update --init --recursive
```

## Generate

```bash
make proto      # runs `buf generate` -> ../generated/ (gitignored)
```

## Upgrading the contract (deliberate pin bump)

A contract change is a **new proto tag**. Bumping is explicit and reviewable:

```bash
cd proto-vendor
git fetch --tags
git checkout proto/v0.2.0
cd ..
make proto
git add proto-vendor && git commit -m "bump proto pin to proto/v0.2.0 + regen"
```

Nothing is ever overwritten behind your back — you choose when to move the pin.
