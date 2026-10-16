# Design

## Clarifications
### Round 1 — 2026-10-14
- Q1 [boundary] grace window: replay of the immediately previous refresh token within 10 s (inclusive) → **returns a fresh access token for the same sid, refresh_token empty (client keeps its current cookie); token stays random, hash-only; no pepper. After 10 s it is reuse → revoke session** (default accepted)
- Q2 [trigger] RefreshSession is called with no Authorization header; public RPC → **yes** (default accepted)
- Reviewer QUESTION: empty/malformed refresh_token → **Unauthenticated, same code as unknown** (orchestrator, follows threat model single-code rule)
