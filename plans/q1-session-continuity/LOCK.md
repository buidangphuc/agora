# LOCK — q1-session-continuity
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-10-14
spec: plans/q1-session-continuity/00-spec.md (Locked 2026-10-14, Amendments from spec-clarify rounds 1-2)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q1-session-refresh-contract | platform-core | [] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 82be5e99af6b | [x] |
| identity-session-refresh | team-identity | [q1-session-refresh-contract] | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 9b4796f21a14 | [x] |
| frontend-session-continuity | team-frontend | [identity-session-refresh] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | a8c72e1b1d6d (re-locked 2026-10-14, was f4446b856121) | [x] |
| frontend-login-return | team-frontend | [frontend-session-continuity] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 2ec945cfd7db | [x] |

## Build order (cross_change_check)
1. q1-session-refresh-contract (contract, sequential)
2. identity-session-refresh
3. frontend-session-continuity
4. frontend-login-return

The whole chain is sequential. There are 2 hot-file pairs:
- `team-frontend/src/features/auth/actions.ts`: frontend-session-continuity, then frontend-login-return.
- `team-frontend/src/middleware.ts`: frontend-session-continuity sets `x-pathname`, and frontend-login-return consumes it.

## Cross-change check
pass (`cross_change_check: OK`).

## Open issues / NEEDS_HUMAN
**Risk routing**
- auth: all 4 slices go to auth-scope-reviewer.
- proto: the contract slice goes to contract-boundary-reviewer.
- pii-light: the refresh-token hash, and order ids in returnUrl. privacy-reviewer at the Feature gate.

**Decisions locked in** (design.md Clarifications in each change)
- The refresh token is random and only its hash is stored. There is no pepper.
- A token replayed within 10 s (inclusive) returns an access token only. A replay after that is treated as reuse and the session is revoked.
- Refresh expiry slides by 30 days. Sessions have an absolute lifetime of 90 days.
- An empty, oversized, unknown, expired, revoked or deleted-user token returns Unauthenticated, all with the same message.
- The frontend calls RefreshSession with no Authorization header. The gateway adds a public forwarder only.
- The RevokeSession denylist expiry is fixed in `team-identity/internal/handler/session.go`.
- Middleware runs on the Node runtime and sets `x-pathname`.
- On a transient refresh error, the cookies are kept. On logout with only the refresh cookie, the frontend refreshes and then revokes.
- `returnUrl` is stripped from GA4 page_location. Buy-now lands on the PDP with no replay.

**Non-blocking reviewer notes**, for the implementer and task-reviewer:
- identity:
  - Treat a token that is not valid base64url as unknown.
  - Apply the 90-day check before the grace-replay decision, and add a scenario for it.
  - Use an injectable clock for the 10 s and 90 d boundaries.
  - The test name `TestRefreshSessionRejectsBadInput` can be renamed.
  - The rate-limit scenario uses the token bucket (`RATE_LIMIT_BURST=40`, 41st call returns ResourceExhausted).
  - p95 < 100 ms is a non-gating target.
- frontend-session-continuity:
  - S15 (logout with only the refresh cookie) is a defensive path, because middleware normally refreshes first. The e2e must force it by failing or skipping the middleware refresh.
  - Confirm that a server action's `cookies()` sees the rotated cookies after a same-request refresh.
  - A legacy access token without `sid`: clear the cookies and make no revoke call.
  - Add S26 to the unit tests in `middleware.test.ts`.
- frontend-login-return:
  - State that the middleware matcher covers every guarded route.
  - `loginPath(x)` drops the parameter when `safeReturnUrl(x) !== x`.
  - Widen the static guard scan regex to catch template-literal and single-quote forms of `redirect("/login`.
  - Split the guard e2e test per variant, including `/favorites`.

**Tooling gaps**
- `decide.py` does not produce a spec-gate decision. Under the system python it fails with a missing yaml module. Under the venv, the kev engine fails with "give exactly one of --state or --part", so every unit escalates to the LLM.
- `validate_plan.py plans/q1-session-continuity` expects parallel-scrum task packets (`wave{N}-task{M}-*.md`) and fails here. The initiative-plan flow does not produce those packets.
- Both gaps are already tracked under ASDLC tooling gaps.

## Re-lock
- 2026-10-14 — frontend-session-continuity: added **VERIFIED BY** metadata lines to 7 unit-only scenarios for spec_sync --strict; no behaviour change. Re-approved by Bùi Đăng Phúc. New digest a8c72e1b1d6d.
