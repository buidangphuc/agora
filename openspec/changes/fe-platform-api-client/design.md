## Context

See proposal.md. ADR-0001 fixed the gateway as the only backend; ADR-0003 (accepted at P4) fixed protobuf over OpenAPI. `team-frontend` already has the interceptor and clients in `src/lib/gateway/` (Connect-ES 1.x, bearer plus request id, anonymous fallback). Its vendored proto copy is older than `platform-core/packages/proto` (listing, identity and chat differ), so it is not the source.

## Goals / Non-Goals

**Goals:** one typed, tested way to call the gateway; failures become one error type; a data-backed reference page an agent can copy; contract drift fails CI.
**Non-Goals:** login flows, refresh, client-side data libraries, RPCs outside listing reads.

## Decisions

1. **Generate from `platform-core` in place.** `packages/api-client/buf.gen.yaml` uses `inputs: directory: ../../../platform-core/packages/proto` with `paths` limited to `platform/common` and `platform/listing`. Alternative considered: vendor a copy (ADR-0001 of platform-core) — rejected for now because the backend contract is moving; one input line switches later. Add services by extending `paths`, not by copying files.
2. **Committed output, drift gate.** Output lands in `src/generated/` and is committed (so CI and consumers need no sibling checkout to build). `scripts/proto-check.mjs` runs `buf generate` into a temp directory and compares trees; any difference, or a missing `../platform-core`, exits non-zero with a clear message. Plugins are the local 1.x `protoc-gen-es` and `protoc-gen-connect-es` with `target=ts`, pinned in devDependencies, so output is deterministic.
3. **Client factory** `createGatewayClients({ token, requestId, anonymousFallback, transport })` returns promise clients for the listing service. `transport` is optional and exists so tests use `createRouterTransport` from `@connectrpc/connect` instead of the network. Default transport is `createConnectTransport` from `@connectrpc/connect-node`, `httpVersion: "1.1"`, base URL from `GATEWAY_URL` (validated as http(s), as in `team-frontend`). The file imports `server-only`.
4. **Interceptors** copied from `team-frontend/src/lib/gateway/auth.ts`: `authInterceptor` (bearer, `x-request-id`) and `anonymousFallbackInterceptor` (public reads only, sits inside the auth interceptor). The client-IP/user-agent forwarding is dropped: it only serves session-creating calls, which are out of scope.
5. **Typed error.** `ApiError` carries `kind` (`unauthenticated | forbidden | not_found | invalid | unavailable | internal`), the Connect `code`, `message`, `requestId`. `toApiError(unknown)` maps `ConnectError` codes to kinds; non-Connect errors become `internal`. Alternative: expose `ConnectError` — rejected, it leaks transport detail into screens. Pages call a small wrapper `gateway(fn)` pattern: catch, convert, and rethrow `ApiError`.
6. **Session helper** `templates/app/lib/session.ts` (server-only): `getToken()` reads cookie `session` through `next/headers`; `requireSession()` redirects to `/` when absent. Used in `(internal)/layout.tsx`. No login page in P4: the cookie is set by whatever sign-in the product adds later (non-goal). The dashboard demo from P3 therefore redirects without a cookie; the README and project.md state this.
7. **Error boundaries.** `app/error.tsx` and `app/(internal)/error.tsx` are client components with a Button reset and a generic message; Next hides server error details in production, so request ids appear only in server logs. `not_found` kind calls `notFound()` in the page, rendered by `app/not-found.tsx`.
8. **Reference feature** at `app/(public)/products/page.tsx` (list via `ListListings`, `status: "published"`, Table) and `products/[id]/page.tsx` (detail via `GetListing`). A view-model mapper converts bigint price to a formatted string; screens never see proto types. Public RPCs use `anonymousFallback: true`.
9. **Boundary rule** in `scripts/check-boundaries.mjs`: files under `templates/` may not import a path containing `/generated/` or a specifier starting with `@connectrpc/` or `@bufbuild/`. Only `packages/api-client` may.
10. **Generated code exclusions.** `biome.json` ignores `packages/api-client/src/generated`; disables and tokens scans skip it; tsc still type-checks it.

## Risks / Trade-offs

- [Reading a sibling directory couples fe-platform to the agora checkout layout] → accepted by the user for now; `proto:check` fails loudly when the sibling is absent; vendoring is the documented exit.
- [Backend contract churn makes `proto:check` red] → that is the intended signal; fix is `npm run proto -w packages/api-client` and commit the diff.
- [Committed generated code inflates diffs] → limited to two proto files' output.
- [Node 22 and Connect 1.x] → same pairing as `team-frontend`, known to work.

## Migration Plan

Additive. Rollback: remove `packages/api-client`, the products routes, session helper, error files, the boundary rule and the `proto:check` step.

## Open Questions

none (ADR-0003 and proto source confirmed by the user 2026-10-29).
