# Design

## Ban and login races

- `SetStatus` locks the user row `FOR UPDATE`; `CreateSession` re-reads the user's status `FOR SHARE` in its own transaction and refuses a suspended or banned user. The two serialise on the user row: a ban that commits first makes the insert fail, and a session that commits first is revoked by the ban. Token scopes come from the status read under that lock, so a restriction cannot race in a `listing.write` token either.
- `LiftExpiredSuspension` is a conditional update. A 0-row result is never read as "lifted": `Login` re-reads the user and re-runs the status gate, so a concurrent login proceeds (status already `active`) and a concurrent ban refuses.

## Assumptions (reviewer recommendation, not a human decision)

- Reinstating to ACTIVE stores the request's new `reason_code` in `status_reason` and clears `status_until`.

## Clarifications

### Round 1 — 2026-10-16

- A14 [Trust] User enforcement: Setting the same status updates reason and until / Reinstating to active revokes nothing: Which SetUserStatus targets revoke sessions? → **RESTRICTED, SUSPENDED and BANNED, including re-setting the same status, revoke all sessions; ACTIVE revokes none. Contract and identity slice must say the same.** (human)
- A15 [Trust] Login enforcement: Expired suspension is lifted: When is an expired suspension lifted? → **Only at the user's next login (listings stay HELD until then); a lift sweeper is a follow-up.** (human)
- A16 [Trust] Login enforcement: Refusal does not leak the reason or note: What does a refused login say? → **One generic message ("tài khoản bị khóa"); no suspended/banned distinction or end time.** (human)
- XS-gateway [Cross-slice] User enforcement edge: Admin reaches SetUserStatus: Which slice owns the gateway forwarder for AccountAdminService? → **identity-user-enforcement (this slice).** (human)
