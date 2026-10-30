# ADR-0003: API client is generated from protobuf, not OpenAPI

Status: accepted, 2026-10-29 (confirmed by the user at the start of P4). Deviates from the planning notes.

## Decision

`packages/api-client` (P4) is generated with buf from the contract in `platform-core/packages/proto`, using Connect-ES, the same toolchain as `team-frontend`. The gateway at :8080 is the only backend the client may call (architecture rule 1). The generated directory is never hand-edited and is blocked by the existing generated-code guard. The client maps failures to one typed error and forwards `x-request-id`, following the pattern in `team-frontend/src/lib/gateway/auth.ts`.

Source of the contract: generation reads `platform-core/packages/proto` in place (restricted to `platform/common` and `platform/listing` in `packages/api-client/buf.gen.yaml`) while the backend contract is still changing. Vendoring a copy later changes one input line. Reopen if the sibling layout stops being guaranteed.

Transport: `@connectrpc/connect-web` (fetch based), not `connect-node` 1.x, which pulls a vulnerable `undici`.

## Why

The gateway speaks Connect; no OpenAPI document exists for it.

## Done-check for P4

Regenerating the client yields no diff.
