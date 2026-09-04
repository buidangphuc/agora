# proto/ — vendored platform contract

This directory holds a **pinned copy of platform-core's proto module** (package
`platform.*`). It is the input to `buf generate` (see `../buf.gen.yaml`), which
writes Go code into `../generated/` (gitignored).

Per **ADR-0001** (pin + local-generate), `platform-core` owns the gRPC contracts
under `packages/proto` and versions them with git tags (`proto/vX.Y.Z`). This
service does **not** consume generated code from platform-core; it vendors the
*proto sources* and generates its own Go code locally. platform-core never writes
into this repo.

## Why a copy (not a submodule) — for now

A vendored copy keeps this repo self-contained and buildable/pushable without a
platform-core remote URL. When platform-core is published, switch to a git
submodule pinned at a tag for auditable upgrades:

```bash
rm -rf proto && git submodule add <platform-core-repo-url> proto
(cd proto && git checkout proto/v0.1.0)   # pin at a tag, never a moving branch
```

## Regenerate

```bash
make proto      # buf generate -> ../generated/ (gitignored)
go mod tidy
```

## Upgrading the contract (deliberate)

A contract change is a **new proto tag** in platform-core. Re-vendor at the new
tag (or bump the submodule pin) and `make proto`. Nothing is overwritten behind
your back — you choose when to move the pin.

## Ownership

This service (team-domain) OWNS `platform.listing.v1.ListingService`. The other
modules here (`common`, `search`, `chat`) are vendored only so the contract
imports resolve during codegen; `search`/`chat` are owned by team-ai.
