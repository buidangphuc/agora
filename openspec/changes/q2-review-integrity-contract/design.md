# Design

## Clarifications
### Round 1 — 2026-10-15
- Wire shape and comment text are taken verbatim from the Locked Contracts section of `plans/q2-review-integrity/00-spec.md`.
- `HideReviewResponse` is intentionally empty: success is the OK status; idempotent re-hide also returns OK.
- No `actor_id` field on the request: the actor comes from the caller's Principal.
