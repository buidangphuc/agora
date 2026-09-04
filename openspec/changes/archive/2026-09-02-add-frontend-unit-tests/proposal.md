## Why

The test pyramid is an hourglass: strong Go backend unit tests and strong `platform-e2e` coverage, but
**`team-frontend` has no unit/component test layer at all** — no runner, no `test` script, zero test
files (quality tooling is Biome + `tsc` only). The frontend holds real logic that E2E exercises only
indirectly and slowly: 9 Server Action files (`src/features/*/actions.ts`) and the gateway data layer
(`src/lib/gateway/*.ts`) that shapes every request/response between the browser and the gateway. A bug
in an action's input mapping or error handling currently has no fast, isolated test to catch it.

This change adds the missing tier: Vitest + React Testing Library, a `test` script, and unit tests for
the Server Actions and gateway client wrappers with the gateway transport mocked. It closes the P0
"frontend Unit/Component test" gap from the QA review without touching product behavior.

## What Changes

- **team-frontend** — add dev deps `vitest`, `@testing-library/react`, `@testing-library/jest-dom`,
  `@testing-library/user-event`, `jsdom` (or `happy-dom`); a `vitest.config.ts` (jsdom env, path aliases
  matching `tsconfig`) and a test setup file; `"test": "vitest run"` + `"test:watch": "vitest"` scripts,
  and fold `vitest run` into `check` (kept as a separate step so lint/typecheck still run standalone).
- **team-frontend** — unit tests for the gateway client wrappers `src/lib/gateway/*.ts`
  (auth, orders, cart, listings, chat, ai, engagement, reviews, addresses, payment) — assert each maps
  inputs to the ConnectRPC client call and normalizes success/error, with the transport/client mocked.
- **team-frontend** — unit tests for the 9 Server Actions `src/features/*/actions.ts`
  (address, assistant, auth, cart, chat, engagement, listing, order, review) — assert each validates
  input, calls the right gateway wrapper, and returns the expected action result / error shape, with the
  gateway layer mocked and `next/*` (cookies, revalidate, redirect) stubbed.
- **CI/quality** — the `test` script is runnable in the existing lint/typecheck pipeline; document how to
  run it in the repo (`README`/`AGENTS` note if the repo has one).

## Non-goals

- **No component/render E2E and no full-page tests** — end-to-end user journeys stay in `platform-e2e`
  (Playwright). This tier is isolated unit tests of actions + client wrappers.
- **No visual/snapshot testing**, no Storybook, no coverage-gate enforcement in CI (a threshold can be a
  later change once a baseline exists).
- **No product/behavior change** and no refactor of the actions or gateway layer beyond what's needed for
  testability (e.g. minor dependency-injection seams only if unavoidable, called out in tasks).
- **No new hooks/util tests beyond** actions + gateway wrappers in this first change (state is currently
  inline `useState`; extracting/ testing hooks is a follow-up).
- **No backend, proto, or gitops changes.**
