## Why

`isSecureRequest` (`team-frontend/src/lib/gateway/session-cookie.ts`) marks the `session` and `agora_refresh`
cookies `Secure` only when the request carries `x-forwarded-proto: https`. A TLS terminator that does not set
that header leaves both cookies non-Secure, so a refresh token that is valid for 30 days can travel over plain
http. `agora_auth_evt` (`AUTH_EVT_OPTS` in `src/features/auth/actions.ts`) never sets `secure` at all.
`NODE_ENV=production` is baked into every image (including plain-http local compose), so it cannot be the switch.

initiative: privacy-audit-hardening

## What changes

- New server-only runtime env `COOKIE_SECURE`, read at request time (never `NEXT_PUBLIC_*`, never baked at build).
  Exactly the string `true` (case-insensitive, surrounding whitespace ignored) forces `Secure`; any other value or
  unset keeps today's rule (`x-forwarded-proto === https`).
- `isSecureRequest(headers)` in `session-cookie.ts` implements that rule; it stays the single source for
  `src/middleware.ts` (silent refresh) and `src/features/auth/actions.ts` (`setSession`, login, register).
- `agora_auth_evt` (set on login and sign_up) now gets `secure` from the same `isSecureRequest` rule (it had none).
  It stays `httpOnly: false` (GTM reads it), `maxAge` 60, `sameSite: lax`, `path: /`.
- `platform-gitops/envs/staging/services/team-frontend.yaml` and `.../prod/services/team-frontend.yaml` set
  `env.COOKIE_SECURE: "true"`. Local compose (`docker-compose.services.yaml`, `platform-e2e/docker-compose.e2e.yaml`)
  and `envs/local` leave it unset.
- `team-frontend/.env.example` documents `COOKIE_SECURE`; `team-frontend/README.md` section 9 "Known gaps" drops the
  x-forwarded-proto gap and the config table lists the variable.
- No proto, RPC or event change. No change to cookie names, lifetimes, httpOnly or sameSite.

## NFR

- Latency: one `process.env` read per call; no measurable change.
- Availability: with `COOKIE_SECURE=true` on a plain-http origin the browser drops Secure cookies and login appears
  broken, so it MUST NOT be set in local compose or the kind `local` env.
- Rollout is a config change: removing the variable restores the previous behaviour (rollback = one values edit).

## Data classification

| Field | Purpose | Basis | Retention |
|---|---|---|---|
| `session` cookie (access JWT) | authentication | contract / legitimate interest (security) | 1 h (`maxAge` 3600), unchanged |
| `agora_refresh` cookie (opaque refresh token) | silent session continuation | same as Q1 session-continuity (DPO item open there) | 30 d (`maxAge` 2592000), unchanged |
| `agora_auth_evt` cookie (`login` or `sign_up` marker, no id) | one-shot GA4 auth event | analytics consent as for the existing GTM events | 60 s, unchanged |

This slice changes only transport protection (the `Secure` attribute); it collects nothing new.

## Threat model

- Assets: refresh token and access JWT in cookies.
- Abuse cases: (1) network attacker on a plain-http hop captures cookies because the proxy omitted
  `x-forwarded-proto`; (2) misconfiguration sets `COOKIE_SECURE=true` on http and locks users out (availability);
  (3) the flag leaks to the browser via `NEXT_PUBLIC_*` (no secret, but it would be build-time and wrong).
- Authz rule: cookie attributes are decided server-side only, from `COOKIE_SECURE` and request headers; a client
  cannot downgrade them (a client-sent `x-forwarded-proto: http` cannot turn `Secure` off when `COOKIE_SECURE=true`).
