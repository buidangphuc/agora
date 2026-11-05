# identity-sso-handoff: one sign-in across buyer and seller apps (G8)

initiative: p10-backend-gaps

## Why
A buyer signed in on the buyer app who clicks "Kênh người bán" lands on the seller login form and signs in again,
because each app has its own cookies and a refresh token cannot be shared (the backend rotates it and treats reuse as
theft, ADR-0006 / D3). Identity needs a one-time hand-off code so the target app can open its own session.

## What changes
- team-identity: `AuthService.CreateHandoffCode` (authenticated user, target buyer or seller),
  `AuthService.PeekHandoffCode` (public, read-only, never consumes) and
  `AuthService.RedeemHandoffCode` (public, never retried by the gateway) as defined in the proto contract slice
  `p10-gaps-contract`. A new table `handoff_codes` stores only the SHA-256 of the 32-byte random code, `target_app`,
  source `sid`, user id, `expires_at = created_at + 60 s`, `redeemed_at`. Redeem runs one transaction in order: atomic consume, check the source session is live and the account ACTIVE, open a NEW session (own sid and refresh token,
  roles read at redeem time, independent of the source session afterwards) for the target app, write the `auth.handoff_redeem` audit entry, and commit only if the audit write succeeded. A loser fails at the consume step and writes no audit entry. Peek returns the
  username and `expires_at` for a valid code and the same fixed UNAUTHENTICATED as redeem otherwise. Mint and redeem write team-audit entries `auth.handoff_mint` / `auth.handoff_redeem` before
  the effect, fail-closed `UNAVAILABLE`. A sweeper purges rows 24 h after `expires_at`.
- team-gateway: expose `CreateHandoffCode` as an authenticated user RPC, `PeekHandoffCode` and `RedeemHandoffCode`
  as public RPCs like `RefreshSession` (redeem never retried, no code in logs).
- fe-platform/packages/session: a `startHandoff` server action (mints on click, never at render, code never in
  markup) and an `/auth/handoff` route handler factory. GET peeks and renders the confirmation page "Tiếp tục với
  tài khoản <username>?" with a button; only the POST from that button redeems server-side, sets that app's cookies
  and redirects (303) to `next` without the code. The 200 confirmation page carries `Referrer-Policy: same-origin` (header and meta) so a same-origin form POST carries the real `Origin` while a cross-origin navigation gets no referrer; every other response carries `Referrer-Policy: no-referrer`. HEAD and
  prefetch never consume.
- The confirmation page is `no-store`, framed by nobody (`Content-Security-Policy: frame-ancestors 'none'`,
  `X-Frame-Options: DENY`), loads no external resource, HTML-escapes and truncates the peeked username, and the code
  travels in the POST body only, never in the form action URL. POST is accepted only from the app's own origin,
  meaning its configured public origin, never the request Host or Forwarded headers. If `Origin` is present it must
  equal the own origin (`null` is refused); otherwise `Sec-Fetch-Site` must be `same-origin`. Anything else is a 303
  to `/login` with no `next`, no redeem call, no cookie and `Referrer-Policy: no-referrer`. A user already signed in
  to the target app still gets the confirmation page; the POST replaces that app's session with the new account. Dwelling more than 60 s on the page falls back to login on the click: intended.
- team-frontend (buyer) and team-frontend-seller: cross-app links call `startHandoff` on click; each app mounts
  `/auth/handoff`. Admin app never mints or redeems.
- Ingress and Next server access logs redact the `code` query of `/auth/handoff` (config in platform-gitops
  ingress annotations and the Next logging config of both apps).
- `fe-platform/docs/gaps.md`: row G8 removed (closed) when both apps are wired.
- Not in scope: `BecomeSeller` (identity-seller-onboarding), SSO into or out of admin.

## NFR
- Redeem p95 < 150 ms locally (also for concurrent-redeem losers); mint p95 < 100 ms locally; peek p95 < 50 ms locally.
- TTL exactly 60 s, inclusive: redeem at `expires_at` succeeds, at `expires_at + 1 ms` fails. A TTL override env `HANDOFF_TTL_OVERRIDE`
  exists for the e2e stack and is honoured only when `ENV=test`; with any other `ENV` and the override set,
  identity fails startup.
- Availability: mint and redeem depend on team-audit; audit down means `UNAVAILABLE`, no code minted or consumed.
- Limits: code is 32 random bytes, base64url without padding (43 chars); redeem rejects any other length without a
  database lookup. Per-user cap of 10 live codes (unredeemed and unexpired only); the 11th mint is `RESOURCE_EXHAUSTED`,
  enforced atomically (concurrent mints cannot exceed 10).

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| handoff code (plaintext) | secret bearer credential | carry sign-in to the other app | contract | never stored; lives in one browser redirect and one RPC; never logged, never in audit metadata, never in traces |
| code SHA-256 | secret-derived | lookup key | contract | purged 24 h after `expires_at` |
| user id, source sid, target app, timestamps | personal (pseudonymous ids) | sign-in hand-off, abuse investigation | contract | purged 24 h after `expires_at` |
| peek response (username, expires_at) | personal | confirmation page "continue as <username>?" | contract | not stored, not logged, rendered once into a `no-store` page |
| audit rows `auth.handoff_*` | personal (actor id) | accountability | legitimate interest, as other audit rows | audit retention, no code or hash in metadata (only target app and result) |
Logs carry no code, no hash, no user id; only result category (`ok`, `rejected`, `audit_unavailable`).

## Threat model
- Assets: the code (grants a session), the source session, the new session.
- Abuse: code theft from history/referrer/log (mitigated: one use, 60 s, redirect strips it, same-origin referrer policy so the code never leaves the app, never logged);
  replay (atomic single use); cross-target use (bound to `target_app`); guessing (256-bit entropy, hash lookup);
  user enumeration or oracle (one fixed UNAUTHENTICATED message for all failures); minting after logout or suspension
  (source session and ACTIVE checked at redeem; ACTIVE at mint); DB leak (only hashes); open redirect via `next`
  (existing `safeNext`); admin escalation (target enum has no admin; admin app has no route).
- Login CSRF (attacker mints a code for their own account and lures the victim into opening the link, so the victim
  is silently signed in as the attacker): mitigated by the confirmation page. GET never consumes; it shows the
  attacker's username ("Tiếp tục với tài khoản <username>?") and nothing is redeemed without the victim's click on
  the POST button. Link scanners, HEAD and prefetch cannot consume the code. Mint happens on click, never at render,
  so the code never sits in page markup or caches.
- Cross-site forged POST (attacker page auto-submits a form with the attacker's code to the victim's browser):
  mitigated by the Origin / `Sec-Fetch-Site` check; a POST with neither header is refused. Clickjacking of the
  confirm button: refused by `frame-ancestors 'none'` and `X-Frame-Options: DENY`. Username injection into the page:
  escaped and truncated.
- Concurrent redeem losers write no audit entry. A commit failure after a successful audit write is accepted: the
  extra `auth.handoff_redeem` entry is an attempt record.
- Peek is a public oracle for username of a valid code only; invalid codes get the one fixed UNAUTHENTICATED message,
  and the code is 256-bit so enumeration is infeasible.
- Accepted: audit down on redeem leaves the code usable within its TTL (nothing was consumed). The resulting
  UNAVAILABLE vs UNAUTHENTICATED difference is an oracle accepted by decision, since only the holder of a valid code
  can trigger it.
- Session independence: the derived session carries the user's roles read at redeem time; after redeem, revoking the
  source session does not end it (and vice versa). Accepted by decision.
- Residual risk: the code is in the `?code=` query of the GET and can reach ingress and Next access logs, browser
  history and the address bar until the POST redirect. Mitigation: ingress and Next server logs redact the `code`
  query on `/auth/handoff`, 60 s TTL, single use, redirect strips it, `Referrer-Policy: same-origin` on the confirmation page and `no-referrer` on every other response.
- Authz: mint requires a user principal (service principals refused `PERMISSION_DENIED`); the code is bound to the
  caller's own `sid` from the token, never from the request; redeem needs no principal, the code is the credential.
