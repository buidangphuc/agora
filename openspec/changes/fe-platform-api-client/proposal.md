## Why

P3 gave agents components, but a screen still cannot show real data: there is no typed way to call the gateway, no rule for where the session token lives, and no reference for a data-backed page. Without one, each agent invents its own fetch code, error handling and auth redirect.

## What Changes

- New `fe-platform/packages/api-client`: Connect-ES clients generated with buf from `platform-core/packages/proto`, restricted to the contract slice the reference feature needs (common, listing). Generated code is committed and never hand-edited.
- A client factory that targets the gateway (`GATEWAY_URL`, default `http://127.0.0.1:8080`) and attaches `authorization: bearer` and `x-request-id` through an interceptor, following the `team-frontend` pattern (copied, not imported).
- One typed `ApiError` (kind, Connect code, message, request id) and `toApiError` that every failure goes through.
- `proto:check` script: regenerates into a temp directory and fails on any diff against the committed output; added to `npm run check`.
- App shell in `templates/app`: server-side session helper (HttpOnly cookie `session`), `requireSession()` guard in the `(internal)` layout, `error.tsx` boundaries for both route groups, `not-found` handling.
- Reference feature `(public)/products` list and `[id]` detail, built only from `@fe-platform/ui`, `@fe-platform/tokens` and `@fe-platform/api-client`.
- New boundary rule: app code must not import generated proto files or `@connectrpc/*` directly.
- ADR-0003 moves from proposed to accepted (confirmed by the user 2026-10-29).
- No change to `team-frontend`.

## Capabilities

### New Capabilities
none (platform template; no service or product behaviour)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New workspace package `packages/api-client`; `templates/app` depends on it.
- New dependencies: `@bufbuild/protobuf`, `@connectrpc/connect`, `@connectrpc/connect-node`, `server-only`; dev: `@bufbuild/buf`, `@bufbuild/protoc-gen-es`, `@connectrpc/protoc-gen-connect-es` (the 1.x line `team-frontend` uses).
- `proto:check` reads `../platform-core/packages/proto` by path; the backend contract is still changing, so generation reads it in place instead of vendoring a copy (user decision 2026-10-29). Switching to a vendored copy later changes one input line in `buf.gen.yaml`.
- Root `npm run check` gains `proto:check`; Biome, tokens and disables scans skip the generated directory.

## Non-goals

Login or register flows, token refresh, a client-side data layer (SWR or React Query), RPCs beyond listing reads, browser-side gateway calls, mocking the gateway as a service, mapping into `team-frontend`.

## Non-functional requirements

- The bearer token is read only on the server and never reaches client bundles (`server-only` on the client factory and session helper).
- Regenerating yields byte-identical output; the check runs in under 20 s locally.
- `npm run check` stays under 60 s locally.

## Data classification

| Data element | Class | Retention | Controls |
|---|---|---|---|
| Session token (cookie `session`) | secret | browser session | read server-side only; never logged; never in a URL |
| `x-request-id` | internal | not stored | random UUID per call unless supplied |
| Listing fields shown (title, price, image keys) | public | not stored | none beyond gateway scopes |

No personal data is stored by this change.

## Threat model

| Threat | Mitigation |
|---|---|
| Token leaks into the browser bundle | `server-only` import on factory and session helper; a test asserts no `"use client"` file imports them |
| Stale token breaks public pages | `anonymousFallbackInterceptor` for public reads only; authenticated calls surface the 401 |
| App bypasses the typed client | boundary rule blocks direct generated/`@connectrpc` imports in app code |
| Contract drift unnoticed | `proto:check` fails on diff in `npm run check` and CI |
