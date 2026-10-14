# Design

## Clarifications
### Round 1 — 2026-10-14
- Q1 [boundary] grace window: replay of the immediately previous refresh token within 10 s (inclusive) → **returns a fresh access token for the same sid, refresh_token empty (client keeps its current cookie); token stays random, hash-only; no pepper. After 10 s it is reuse → revoke session** (default accepted)
- Q2 [trigger] **call RefreshSession without Authorization header** (default accepted)
- Q4 [trigger] **middleware runs on Node runtime (`runtime: "nodejs"`, Next 15.5) and reuses the connect-node client**; no new dependency (default accepted)
- Q5 [error] (a) any RefreshSession error other than Unauthenticated → **render anonymous, keep both cookies unchanged**; Unauthenticated → clear both. (b) logout with refresh cookie only → **RefreshSession to obtain sid, then RevokeSession; on failure clear cookies** (default accepted)
- Q7 [trigger] **middleware sets `x-pathname` request header** (consumed by frontend-login-return) (default accepted)
