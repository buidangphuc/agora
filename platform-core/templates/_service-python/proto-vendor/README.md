# proto-vendor/

The platform proto contracts, vendored as a **git submodule** pinned to a
tagged release of `platform-core`. This is the consumer half of ADR-0001:
`platform-core` never writes generated code into this repo — we vendor the
`.proto` source and run `buf generate` ourselves into a local, git-ignored
`generated/` directory.

## One-time: add the submodule

Replace the URL with your `platform-core` remote. The proto module lives at
`packages/proto` inside `platform-core`.

```bash
git submodule add <platform-core-git-url> proto-vendor/platform-core
cd proto-vendor/platform-core
git checkout proto/v0.1.0    # placeholder tag — pin an explicit release, never a branch
cd -
git add .gitmodules proto-vendor/platform-core
git commit -m "chore(proto): vendor platform-core proto @ proto/v0.1.0"
```

`buf.gen.yaml` and the `proto` Makefile target read the proto files from the
submodule's `packages/proto` module.

## Fresh clone

```bash
git submodule update --init --recursive
```

## Bumping the pinned version

```bash
cd proto-vendor/platform-core
git fetch --tags
git checkout proto/v0.2.0    # the new release tag
cd -
git add proto-vendor/platform-core
git commit -m "chore(proto): bump platform-core proto to proto/v0.2.0"
make proto                    # regenerate stubs against the new contracts
```

## Generated layout & the `platform` stdlib-shadowing caveat

Proto packages are `platform.<domain>.v1`, and buf generates with source-relative
paths, so stubs land under `generated/platform/<domain>/v1/` and import each
other absolutely (`from platform.common.v1 import common_pb2`). `make proto` +
`src/server.py` put `generated/` on `sys.path` so those imports resolve.

The catch: the generated top-level package is literally `platform`, which
**shadows Python's stdlib `platform` module** whenever `generated/` is on
`sys.path` — third-party code doing `import platform` can break. To harden,
rewrite the generated imports to relative with **protoletariat** so the tree can
live under a private `generated` package (no top-level `platform` on sys.path):

```bash
protol --create-package --in-place --python-out generated buf --config-path buf.gen.yaml
```

The Makefile's optional `proto-relative` target runs this. See the top-level
`README.md` and the caveat block in `src/server.py` for the full explanation.

## Rules

- Pin to an explicit **tag**, never a moving branch — proto is an API contract.
- Do not edit anything under `proto-vendor/` by hand; it is upstream-owned.
- Do not commit `generated/` — it is a build artifact (see `.gitignore`).
