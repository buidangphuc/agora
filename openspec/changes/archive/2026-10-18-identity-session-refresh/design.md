# Design

## Clarifications
### Round 1 — 2026-10-14
- Q1 [boundary] grace window: replay of the immediately previous refresh token within 10 s (inclusive) → **returns a fresh access token for the same sid, refresh_token empty (client keeps its current cookie); token stays random, hash-only; no pepper. After 10 s it is reuse → revoke session** (default accepted)
- Q2 [trigger] frontend sends no Authorization header; **no gateway bearer-skip exception**, gateway only adds the public forwarder (default accepted)
- Q3 [error] **RevokeSession denylist expires_at = now + access TTL** — fixed in this slice (default accepted)
- Q6 [boundary] (a) owning user deleted → **Unauthenticated + revoke**; (b) token older than one rotation → **Unauthenticated, no revoke**; (c) lost response retried after 10 s → **reuse, hard logout accepted** (default accepted)

### Round 2 — 2026-10-14
- Q9 [boundary] absolute session lifetime → **90 days from session created_at; after that RefreshSession returns Unauthenticated (same generic error) and the session is revoked** (human)
- Reviewer BLOCKING: empty or oversized (>256 B) refresh_token → **Unauthenticated, same generic message as unknown** (settled by contract Clarification; orchestrator)
- Reviewer QUESTION: grace replay also loads the current user → **deleted user gets Unauthenticated + revoke; scopes re-derived** (follows Q6a; orchestrator)

### Round 3 — 2026-10-14
- Revoke clears both hashes, so any token of a revoked session (including a prev-hash replay inside the 10 s grace window) takes the unknown-token path: `Unauthenticated`.
- Atomic revoke/outbox failure maps to `Internal`.
- Absolute 90-day lifetime is measured from `sessions.created_at`, independent of the sliding 30-day refresh expiry.
