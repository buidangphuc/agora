## 1. api-client package and generation

- [x] 1.1 Create `fe-platform/packages/api-client` (package.json, tsconfig, `buf.gen.yaml` reading `../../../platform-core/packages/proto` with `paths` for common and listing, `proto` script); add buf and 1.x protoc plugins as devDependencies; verify `npm run proto -w packages/api-client` writes `src/generated/` and `tsc --noEmit` passes
- [x] 1.2 Add `scripts/proto-check.mjs` (regenerate to temp dir, diff against committed output, clear error when `../platform-core` is missing) and a root `proto:check` script; verify it passes, then fails after a hand edit of a generated file and after adding a field to a temp copy of the proto, and passes again after revert
- [x] 1.3 Exclude `src/generated` from Biome, the disables scan and the tokens scan; verify `npm run lint` and `npm run check` ignore it while `tsc` still covers it

## 2. Client, interceptors and typed error

- [x] 2.1 Add `ApiError` and `toApiError` (kinds: unauthenticated, forbidden, not_found, invalid, unavailable, internal); verify a test table maps every Connect code to a kind and non-Connect errors to `internal`
- [x] 2.2 Add interceptors (bearer, `x-request-id`, anonymous fallback) and `createGatewayClients` with `server-only` and `GATEWAY_URL` validation; verify tests with `createRouterTransport`: header set and reused id, fallback retries once without bearer only on Unauthenticated, authenticated call surfaces the error, bad URL rejected
- [x] 2.3 Add index exports and the `server-only` scan test (no `"use client"` file imports the factory or session helper); verify it goes red on a seeded violation and green after revert

## 3. App shell

- [x] 3.1 Add `templates/app/lib/session.ts` (`getToken`, `requireSession`) and guard `(internal)/layout.tsx`; verify with a test that a missing cookie redirects and a present one passes the token through
- [x] 3.2 Add `app/error.tsx`, `app/(internal)/error.tsx` and `app/not-found.tsx` using `@fe-platform/ui`; verify they render under axe with zero violations
- [x] 3.3 Add the boundary rule (templates must not import `/generated/`, `@connectrpc/*`, `@bufbuild/*`); verify `checks.test.mjs` cases for allowed and blocked imports and that a seeded violation fails `npm run boundaries`

## 4. Reference feature

- [x] 4.1 Add view-model mapper and `(public)/products` list page (Table, empty state, error via boundary); verify mapper tests (bigint price, empty list) and that the page imports only shared packages
- [x] 4.2 Add `(public)/products/[id]` detail page with `not_found` handling; verify a unit test that `not_found` kind triggers `notFound()`
- [x] 4.3 Verify `npm run build -w templates/app` passes with the new routes

## 5. Docs and done-condition

- [x] 5.1 Set ADR-0003 status to accepted (2026-10-29) with the in-place proto source note; update `fe-platform/AGENTS.md`, `docs/agents/project.md` (check order with `proto:check`, layout, cookie `session`, no login in P4) and `docs/agents/frontend.md` (data fetching rule: only through api-client)
- [x] 5.2 Done-condition: regenerate shows no diff, `npm run check` exits 0 after a fresh `npm ci`, no new disable comments, `npm audit` clean, `openspec validate fe-platform-api-client --strict` passes
- [ ] 5.3 e2e track: none (no user-facing capability, no FEATURES.yaml entry); confirm `repo-doctor` reports no new drift
