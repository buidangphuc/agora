# Vendored proto contract

This directory is a **pinned copy of platform-core's proto module** (`packages/proto`).
Per platform ADR-0001, consumer repos vendor the contract and generate stubs
locally — platform-core never writes into this repo.

- Source: `platform-core/packages/proto/platform/**`
- Regenerate Python stubs after updating: `make proto`
  (runs `scripts/gen_proto.py` → `app/transport/grpc/_pb/`, import-rewritten so the
  `platform.*` package never shadows Python's stdlib `platform`).

To upgrade the contract: copy the new proto from platform-core at the desired tag
(e.g. `proto/v0.1.0`), then `make proto`. Prefer a git submodule pinned at a tag
once platform-core is pushed, instead of a plain copy.
