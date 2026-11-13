## Context

team-identity owns users, sessions, addresses, handoff codes, the outbox and a session sweeper
(`internal/identity/session_sweeper.go`). Contracts (PrivacyService, `UserErased`, `USER_STATUS_DELETED`,
obligation RPCs) are fixed by slice `trust-legal-contract`; decisions are locked in `plans/trust-legal/00-spec.md`.

## Decisions

- State on the user row: `deletion_requested_at` and `erase_after`, both null when nothing is pending. Pending state
  does not block login, so the user can sign in and cancel. Status stays as is until erasure.
- Request flow, in order: principal check, password check, obligations (order then payment, each single attempt,
  2 s timeout), then one transaction that sets the timestamps and revokes the other sessions. Any obligation call
  error maps to `UNAVAILABLE` and nothing is written. The blocker message names the category only (open orders,
  open returns, wallet balance, pending payouts), never ids or amounts.
- A second request while one is pending is idempotent: it returns the existing `erase_after` and does not extend it.
  It still requires the password and still re-checks nothing else.
- Cancel clears both timestamps; with nothing pending it fails `FAILED_PRECONDITION`.
- Eraser is a store-level transaction taking `SELECT ... WHERE erase_after <= now AND status <> 'deleted'
  FOR UPDATE SKIP LOCKED` one user at a time. Within it: update users (username `deleted_<16 hex random>`,
  `password_hash = ''`, `roles = '{}'`, status `deleted`, timestamps null), delete from `user_addresses`,
  `sessions`, login history, `handoff_codes`, `password_reset_tokens`, insert the `UserErased` outbox row through the
  existing outbox writer (key = user id, roles captured before clearing). Consent and seller-terms rows are not
  touched.
- Login, refresh and handoff exchange treat status `deleted` as unknown user (same error and timing path as a
  missing username), and `ScopesForUser` returns no scopes for `deleted` (fail closed).
- Sweeper config: interval default 1h, batch 100; started beside the session sweeper in `cmd`; stops on context
  cancel.
- Gateway: PrivacyService forwarder is shared with sibling slices; this slice adds the three methods to the route
  policy as signed-in-user-only and keeps obligation RPCs 501.

## Risks

- Obligations are checked only at request time; an order opened during the 48 h window is not re-checked (see open
  issue in the slice report). The eraser itself does not call order or payment.
- Migration numbering (next free after `0021`) may collide with `identity-legal-consent`; the later-built slice
  takes the next number.
