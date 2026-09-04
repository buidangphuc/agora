# proto-vendor/ — the pinned proto module

This directory is a **git submodule** pointing at `platform-core`'s proto module,
**pinned at a tag**. It is the codegen input for `make proto` (see `buf.gen.yaml`).
This is the consumer half of ADR-0001 (pin + local-generate): `platform-core`
never writes generated code into this repo — you vendor the contract and generate
it here.

## First-time setup (per clone)

Add the submodule pinned at a proto tag (placeholder tag shown — use the real one):

```bash
git submodule add https://your-host/platform-core.git proto-vendor/_platform-core
cd proto-vendor/_platform-core && git checkout proto/v0.1.0 && cd -
git add .gitmodules proto-vendor
```

`buf.gen.yaml` points `inputs.directory` at `proto-vendor`. Make sure the proto
module root (the directory containing `buf.yaml` and the `platform/common/`,
`platform/listing/` package dirs) is what buf sees — if the submodule nests the module under a
subpath, point `inputs.directory` at that subpath instead.

After checkout on a fresh clone:

```bash
git submodule update --init --recursive
```

## Generating

```bash
make proto   # buf generate → ./generated  (gitignored; never hand-edit)
```

## Upgrading the contract (deliberate pin bump — ADR-0001)

A contract change is a NEW proto tag. Bump the pin explicitly in a reviewable PR:

```bash
cd proto-vendor/_platform-core
git fetch --tags
git checkout proto/v0.2.0     # the new tag
cd -
git add proto-vendor
make proto                    # regenerate against the new contract
```

Nothing is overwritten behind your back; you choose when to move.

> Placeholder tag `proto/v0.1.0` is used throughout — replace with the tag your
> platform-core actually publishes.
