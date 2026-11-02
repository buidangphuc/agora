## Context

See proposal.md. Facts used: identity roles map to scopes in `team-identity/internal/authz/scopes.go` (buyer: read scopes; seller adds `listing.write`; admin adds `admin` and `order.admin`), the JWT carries `scopes`, `sub`, `name`, `exp`; `RefreshSession` rotates and revokes on reuse (10 s grace). `team-frontend/src/middleware.ts` refreshes with the `agora_refresh` cookie on each request.

## Decisions

1. **Package layout** (`src/`): `config.ts` (presets and `defineSession`), `token.ts`, `cookies.ts`, `middleware.ts`, `server.ts` (guards and cookie writers), `refresh.ts` (default gateway refresh), `index.ts`. Alternative: split into entry points per runtime; rejected for now, one entry with `next/server` and `next/headers` imported only where used and lazily from server helpers.
2. **Config shape:** `{ app, cookies: { access, refresh }, hostPrefix, sameSite, accessMaxAge, refreshMaxAge, requiredScopes, loginPath, forbiddenPath }`. `cookieNames(cfg, secure)` returns names with `__Host-` when `hostPrefix && secure`. `__Host-` forces `path=/`, `secure`, no `Domain`; the buyer preset keeps `hostPrefix: false` so existing sessions survive.
3. **Never `Domain`:** options builders do not accept a domain; a test asserts the option object has no such key.
4. **Admin hardening in the preset:** `sameSite: "strict"`, access cookie 30 minutes, refresh cookie 8 hours; these are cookie lifetimes, the token's own expiry still comes from identity.
5. **Middleware:** `createSessionMiddleware(cfg, { refresh, clientContext })`; `refresh(refreshToken, ctx)` returns `{ token, refreshToken }` or throws a `ConnectError`-like error with a numeric `code`; `Code.Unauthenticated` (16) and `PermissionDenied` (7) clear both cookies, anything else is treated as transient. The Connect `Code` enum is not imported; codes are compared by number via an exported constant so the package needs no `@connectrpc/connect` dependency. Matcher and `runtime` stay in the app's `middleware.ts`.
6. **Default refresh** `gatewayRefresh({ gatewayUrl })` uses `createGatewayClients` from `@fe-platform/api-client` and its `auth.refreshSession`.
7. **Server helpers** (`createSessionServer(cfg, clientsFactory)`): `getToken`, `getPrincipal`, `requireSession(next)` (redirects to `loginPath?next=`), `requireScopes(next)` (redirects to `forbiddenPath` when the token lacks a required scope), `guarded`/`attempt` as in the template but redirecting unauthenticated users to the login path (the middleware has already tried a refresh), `setSession`, `clearSession`. `next` is restricted to same-origin paths by `safeNext`.
8. **Tests** use the fake-identity pattern: an in-memory identity that rotates refresh tokens and revokes a session when an already-used token is presented, to prove (b); a cookie jar keyed by name to prove (a) and (e); presets plus `checkScopes` for (c); the middleware cases for (d).
9. **Dependency direction:** `session` depends on `api-client` (types and default refresh) and peers on `next`/`react`; it never imports `ui`, `tokens` or an app. The boundary check already forbids importing apps; a new rule is not needed.
10. **Vendoring:** `scripts/vendor.mjs` gets a fourth entry (`fe-platform-session.tgz`); `team-frontend` adds the dependency, `transpilePackages` and a smoke import.

## Risks / Trade-offs

- [The extracted middleware could diverge from team-frontend's] -> tests mirror each branch of the existing file; swapping `team-frontend` over is a separate change with its own e2e run.
- [`next/headers` cookies API differs between Next 15 and 16] -> the package only uses the async `cookies()` form already used by the template.
- [Admin lifetimes (30 min, 8 h) are guesses] -> constants in the preset, overridable per deployment; recorded in the ADR follow-up if the product wants different values.
- [`order.admin` is held only by admin; `listing.write` is also held by admin] -> an admin passes the seller scope check; harmless because the gateway enforces real permissions and the seller app is not reachable from admin navigation.

## Open Questions

- Whether the seller app should reject admins (would need a negative scope rule); not needed now.
