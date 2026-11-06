## Why

Referral attribution (user story 2) and the new-user voucher (user story 4) both start at signup. Today
`IdentityService`/`AuthService.Register` stores the user with a plain INSERT (`PostgresUserRepository.Create`) and
announces nothing, and `RegisterRequest` has no place for a referral code. team-referral and team-promotion must learn
about every new account asynchronously (decision D1), without Register depending on either service.

initiative: q7-q37-referral-welcome

## What Changes

- **team-identity**
  - `Register` accepts the optional `RegisterRequest.referral_code` (contract slice, field 4). The value is trimmed and
    bounded: after trimming, the code must match `^[A-Za-z0-9_-]{1,32}$` (case kept; real codes are `REF` + 8 hex). A value outside those bounds is dropped (treated as empty). The
    code never fails Register and never changes its response.
  - The user insert and one `UserRegistered{user_id, referral_code, registered_at}` outbox row are written in the same
    database transaction (`identity_outbox_events`, aggregate type `User`, key = user id, envelope type
    `platform.identity.v1.UserRegistered`, event id = outbox row id, so redelivery carries a stable id). A failed insert
    (duplicate username) writes no outbox row.
  - A builder for the envelope is added next to the existing builders in `internal/events/publisher.go`.
  - Logs and the relay redaction never carry the raw user id or the referral code.
- **team-gateway (Register)**: the public `AuthForwarder.Register` forwards the whole `RegisterRequest`, so the new field passes
  through; add a test that pins it. No route, policy or scope change (Register stays public).
- **team-identity (GetMyAccount)**: implement the new `AuthService.GetMyAccount` (contract slice). It takes no request
  fields and returns `{user_id, created_at}` for the calling principal only; the id comes from the verified Principal,
  never from the request. Anonymous callers get `UNAUTHENTICATED`. The frontend uses it for the 14-day account-age rule.
- **team-gateway (GetMyAccount)**: route `AuthService.GetMyAccount` as an authenticated procedure for any signed-in
  user (no role or scope beyond a valid Principal); anonymous callers are refused with 401 at the edge.
- Proto fields and the `UserRegistered` message come from slice `q7-referral-welcome-contract`; this slice adds no proto.

Repos touched: team-identity, team-gateway. No new RPC beyond `GetMyAccount`, which is defined by the contract slice.

## Capabilities

### New Capabilities
- `identity-user-registered`: Register carries an optional referral code and announces every new account on
  `identity.events`.

### Modified Capabilities
- None.

## Non-goals

- Validating that the code exists or belongs to another user (team-referral decides, emitting `ReferralRejected`).
- Emitting `UserRegistered` for the seeded admin account or any path other than `Register`.
- Cookie or `?ref=` handling (frontend-referral-invite).
- Consuming the event (referral-attribution-reward, promotion-new-user-vouchers).

## NFR

- Register p95 latency adds no more than 10 ms over the pre-change baseline on the local stack (one extra outbox insert in the same transaction, no network call added). `GetMyAccount` is a single primary-key read, p95 at most 50 ms on the local stack.
- Register availability does not depend on Kafka, team-referral or team-promotion: the outbox relay publishes after
  commit. Relay delivery is at least once.
- Limits: `referral_code` at most 32 characters after trim, charset `[A-Za-z0-9_-]`; anything else is dropped, never an error.

## Data classification

| field | where | purpose | basis | retention |
|---|---|---|---|---|
| `referral_code` | `UserRegistered` payload, outbox row | attribute the signup to an inviter | contract (referral programme the user opted into by following an invite) | outbox rows are purged after `OUTBOX_PURGE_RETENTION` (default 168h, sweep every `OUTBOX_PURGE_INTERVAL`, default 1h); the code is kept downstream with the referral row |
| `user_id` | `UserRegistered` payload and Kafka key | tell referral and promotion which account is new | contract | `identity.events` has no per-topic override in `platform-gitops/platform/infra/redpanda.yaml`, so the broker default retention applies (Redpanda `log_retention_ms`, 1 week) |
| `registered_at` | `UserRegistered` payload | referral account-age window | contract | same broker-default retention as above |

The username and password hash are never part of the event. Logs use `principal_fp`/`key_fp` fingerprints only. DPO
sign-off is tracked at initiative level (NEEDS_HUMAN).

## Threat model

- Assets: the integrity of referral attribution, availability of signup, privacy of the user id and referral code.
- Abuse cases:
  - A client sends an oversized or hostile `referral_code` to break Register or inject into logs, topics or SQL. The
    code is length and charset bounded, bound as a parameter, and dropped when invalid.
  - A client probes whether a code exists through Register. Register never reveals code validity: the response is
    identical with a valid, unknown or invalid code.
  - A client forges the event. Only team-identity writes `identity.events`; Register has no caller-supplied user id.
  - Referral or Kafka outage blocks signup. The outbox decouples them.
- Authz rule: Register stays public (no Principal) as today; it creates only a buyer or seller account per the existing
  role rules. `UserRegistered` is internal to the event bus and is not exposed at the gateway.
- `GetMyAccount` exposes only the caller's own `user_id` and `created_at`; there is no request field to name another user.
- Register ignores any client-supplied user id or role escalation: the user id is generated server-side and the role is
  normalised (admin is not self-assignable), pinned by a scenario below. The two remaining spec_lint PERM warnings (on the
  event and log requirements) are accepted: those requirements concern an internal event and logs, not a user-owned
  resource.
