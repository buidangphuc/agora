## Why

initiative: trust-legal

A signed-in user cannot download the personal data the platform holds about them: identity has no export RPC
(`identity.proto` AuthService has none, `PrivacyService` does not exist yet). Data-subject access is part of Q24
(trust#10) of the 2026-10-14 PM review and the initiative goal "download their account data". This slice implements
`PrivacyService.ExportMyData` (declared by `trust-legal-contract`) in team-identity and routes it at the gateway.
The `/account/privacy` page that calls it belongs to `frontend-account-privacy`.

## What Changes

- team-identity: `PrivacyService.ExportMyData` returns one JSON document (schema version 1) built synchronously
  from identity-held data only: account row, addresses, sessions, login history, consent records, seller-terms
  acceptance, and account-deletion state. It is generated on demand, never stored, size at most 1 MiB.
- team-identity: new read-only repository calls for the export; no migration, no new table, no new dependency.
- team-gateway: forward `PrivacyService.ExportMyData` for signed-in users (reads are retried like other reads) and
  set `Cache-Control: no-store` on the response. Shares `team-gateway/internal/edge/auth.go` with the two other
  identity slices; this slice is serialised after them.
- Scope limit (locked): no fan-out to other services; orders, reviews and chats are only linked from the page.
- `initiative: trust-legal`

## Capabilities

### New Capabilities
- `identity-data-export`: the export RPC, its content, size and latency limits, authorisation and caching rules.

### Modified Capabilities
none (the deltas of `identity-legal-consent` and `identity-account-deletion` own the other `PrivacyService` RPCs).

## Impact

- Code: `team-identity/internal/rpc` (new `export.go`), `team-identity/internal/store` (read methods),
  `team-identity/internal/authz` (coverage list), `team-gateway/internal/edge` (forwarder, header).
- Depends on: `identity-account-deletion` (deletion state and consent repository exist; `PrivacyService` is registered).
- Used by: `frontend-account-privacy`.

## NFR

- Latency: `ExportMyData` p95 at most 500 ms for an account with 20 addresses, 200 sessions and the login-history cap
  below, measured at team-identity.
- Size: response `data` at most 1 MiB (1,048,576 bytes), enforced by bounding the large collections, never by failing.
- Availability: same as team-identity reads; a database error returns an error, never a partial file.
- Limits: login history capped at the newest 1000 events; sessions at the newest 1000; consent records all.
- Caching: `Cache-Control: no-store` on the edge response; nothing is cached or persisted server-side.

## Data classification

| Field exported | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| username, user id, roles, created_at, status | personal | data-subject access (Decree 13/2023 and 2026 PDP law, stricter applies) | legal obligation to answer access requests | none: not stored; the export is built per request |
| addresses (recipient name, phone, street, ward, district, city) | personal | same | same | none |
| sessions and login history (device, IP, user agent) | personal | same | same | none |
| consent records, seller-terms acceptance | personal, proof of consent | same | same | none |
| deletion request and `erase_after` | personal | same | same | none |

Never exported: password hash, refresh tokens, handoff codes, other users' data, internal enforcement notes
(`status_reason`). The exported file leaves the server only in the RPC response; it is not logged, not stored, not
cached. Legal confirmation of the applicable instruments and the request deadline is a NEEDS_HUMAN item of the initiative.

## Threat model

- Assets: the full set of personal data above, and the account's credentials (which must never leak into the file).
- Abuse cases: (1) another user's data requested by guessing an id: the request has no user id field and the id comes
  only from the principal; (2) anonymous or service callers reading exports; (3) a stolen access token used to
  harvest the file: the export contains no credential, no password hash, no refresh token, so it gives no
  account takeover step beyond what the token already allows; (4) the file cached by a proxy or browser:
  `no-store`; (5) the file leaking through logs: only a count of records and the byte size are logged, never contents
  or raw ids; (6) denial of service through huge histories: collections are capped and the size is bounded.
- Authz rule: USER principal only; anonymous is UNAUTHENTICATED; service principals are PERMISSION_DENIED; the
  subject is always the principal's own user id. Account status does not matter for the read: a user with a pending
  deletion can still export until erasure.
