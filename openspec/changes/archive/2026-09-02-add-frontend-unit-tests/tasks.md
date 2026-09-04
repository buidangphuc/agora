# Tasks

## 1. Code — team-frontend (test harness)
- [x] Add dev deps: `vitest`, `@testing-library/react`, `@testing-library/jest-dom`,
      `@testing-library/user-event`, `jsdom` (or `happy-dom`). (Installed via `npm install -D`; registry
      reachable. Used `jsdom`. RTL added per scope though no component/render tests exist in this tier.)
- [x] `vitest.config.ts`: `environment: 'jsdom'`, `globals: true`, `setupFiles`, and path aliases
      mirroring `tsconfig.json` so `@/`-style imports resolve in tests. (Also aliases `server-only` → its
      own `empty.js`, since every gateway/action module `import "server-only"` and the default entry throws
      outside an RSC bundle.)
- [x] `vitest.setup.ts`: import `@testing-library/jest-dom`; provide default mocks/stubs for `next/headers`
      (cookies — backed by an in-memory jar so getToken/setSession round-trip), `next/cache`
      (revalidatePath/Tag), and `next/navigation` (redirect/notFound).
- [x] `package.json` scripts: `"test": "vitest run"`, `"test:watch": "vitest"`; `check` is now
      `biome check . && tsc --noEmit && vitest run`; `typecheck` left standalone.

## 2. Code — team-frontend (gateway wrapper tests)
- [x] Unit-test each `src/lib/gateway/*.ts` wrapper (auth, orders, cart, listings, chat, ai, engagement,
      reviews, addresses, payment) with the ConnectRPC client/transport mocked: asserts RPC selection,
      request mapping, and success/error normalization. No real network. (10 co-located `*.test.ts`;
      `makeClients`/`getToken` mocked via `vi.mock`. auth.ts is the interceptor, so its test asserts
      bearer + x-request-id header behavior.)

## 3. Code — team-frontend (Server Action tests)
- [x] Unit-test the 9 `src/features/*/actions.ts` (address, assistant, auth, cart, chat, engagement,
      listing, order, review) with the gateway layer mocked and `next/*` stubbed: input validation, correct
      wrapper called with mapped inputs, and returned result/error shape. Cover at least one success and one
      failure path per action.
- [x] Introduce a testability seam only if unavoidable (prefer module mocking over refactor); note any such
      seam here. (No source seam introduced — pure `vi.mock` module mocking of `@/lib/gateway/*`,
      `@/lib/gateway/client`, and `next/*`. Zero product-code changes.)
- [x] `npm test` green locally. (19 test files, 114 tests passing; `npm run check` also green.)

## 4. E2E (platform-e2e)
- [ ] **None — internal frontend quality tier, non-user-facing.** No `FEATURES.yaml`/`.feature`; user
      journeys remain in platform-e2e. Verification is `npm test` (Vitest) in tracks 1–3. Recorded so the
      archive gate reads "no scenarios", not "missing scenarios".

## 5. Archive
- [x] `npm test` green; `check` runs lint + typecheck + tests; no product behavior changed.
- [x] Sync the `frontend-quality` spec delta into `openspec/specs/frontend-quality/spec.md`. (New capability spec created.)
- [x] Archive the change — moved to `changes/archive/2026-09-02-add-frontend-unit-tests/`.
      (openspec CLI not installed; archived manually in the established format.)
