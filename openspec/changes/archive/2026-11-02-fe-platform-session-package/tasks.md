## 1. Package skeleton and token helpers

- [x] 1.1 Create `packages/session` (package.json with peer deps, tsconfig, index) and `token.ts` (`tokenState`, `isUsableToken`, `decodePrincipal`); verify tests for usable, expired, malformed and scope decoding
- [x] 1.2 Add `config.ts` presets (buyer, seller, admin) and `cookies.ts` (`cookieNames`, option builders, `isSecureRequest`); verify tests: names differ per app, `__Host-` only when secure and enabled, admin is strict, no `Domain` key

## 2. Middleware and server helpers

- [x] 2.1 Implement `createSessionMiddleware` and `refresh.ts` default; verify tests for every branch of the existing middleware (usable passes, stripped when no refresh, refresh success rewrites both, unauthenticated clears both, transient keeps refresh)
- [x] 2.2 Implement server helpers (`requireSession`, `requireScopes`, `guarded`, `attempt`, `setSession`, `clearSession`, `safeNext`); verify tests including redirects and own-cookie-only clearing

## 3. Behaviour tests from the plan

- [x] 3.1 Cross-app tests (a) two apps on one host never overwrite each other's cookies, (b) concurrent refresh in two apps with a rotating, reuse-revoking fake identity revokes neither, (c) buyer rejected by seller and admin presets, (e) signing out of one app leaves the others; verify each fails when the property is broken on purpose (shared names, shared refresh token)

## 4. Distribution and docs

- [x] 4.1 Add the package to `scripts/vendor.mjs` (+test), run `npm run vendor`, add the dependency, `transpilePackages` entry and smoke test to `team-frontend`; verify `npm ci` from a clean copy, `npm run check` and a Docker build
- [x] 4.2 Changeset (minor, new package), notes in `docs/agents/project.md`, SSO hand-off row in `docs/gaps.md`; verify `release:status`
- [x] 4.3 Done-condition: fe-platform and team-frontend `npm run check` green, `npm audit` clean, `openspec validate fe-platform-session-package --strict` passes, `repo-doctor` shows no new drift
