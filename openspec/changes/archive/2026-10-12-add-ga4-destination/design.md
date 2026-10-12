# Design

## Clarifications
### Round 1 — 2026-10-12
- Q1 [permission] Pseudonymous GA4 user_id: the frontend's `getPrincipal()` decodes the session JWT without a signature check, so a forged cookie returns HMAC(key, any sub) (auth-scope-reviewer BLOCKING on task 1.2). Where does the identity endpoint get a verified principal? → **From a new gateway edge endpoint `GET /api/session/principal` (plain HTTP edge handler, no proto change) that verifies the bearer with the existing JWKS verifier + revocation and returns 200 `{principalId}` no-store, else 401; the identity route calls it with `getToken()` and answers 204 on 401 or error** (human)
