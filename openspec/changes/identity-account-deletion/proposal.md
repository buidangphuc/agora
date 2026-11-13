## Why

A signed-in user cannot delete their account today: `identity.proto` has no deletion RPC, and no service erases
personal data when a user leaves (initiative `trust-legal`, user story 3, backlog Q24). This slice delivers the
deletion lifecycle inside team-identity: request with password re-authentication, a 48 h cancel window, a sweeper
that erases due accounts and publishes `UserErased`, and the gateway routes for the three lifecycle RPCs.
Consumers of `UserErased` are other slices (`commerce-user-erasure`, `content-user-erasure`, `domain-seller-erasure`,
`analytics-user-erasure`, `verification-kyc-documents`).

initiative: trust-legal

## What changes

- team-identity implements `PrivacyService.RequestAccountDeletion`, `CancelAccountDeletion`,
  `GetAccountDeletionStatus` (messages and the service come from slice `trust-legal-contract`; `RecordConsent` and
  `ListMyConsents` come from `identity-legal-consent`, `ExportMyData` from `identity-data-export`).
- Migration adds `users.deletion_requested_at` and `users.erase_after` (nullable) and allows status `deleted`
  (`USER_STATUS_DELETED = 5`) in `users_status_check`.
- `RequestAccountDeletion`: verifies the password like `ChangePassword`, calls `OrderService.GetUserObligations`
  and `PaymentService.GetPayoutObligations` with the service principal (fail closed), then sets
  `deletion_requested_at = now`, `erase_after = now + 48 h` and revokes every session except the caller's.
- `AccountEraser` plus an erasure sweeper (mirrors `internal/identity/session_sweeper.go`): for each account with
  `erase_after <= now` it runs one transaction that anonymises the username to `deleted_<random>`, clears the
  password hash and roles, deletes addresses, sessions, login history, handoff codes and password-reset tokens,
  keeps consent records and seller-terms acceptance, sets status `deleted`, clears the deletion timestamps and
  writes `UserErased` to the identity outbox.
- `Login`, `Refresh` and handoff exchange refuse a `deleted` account like an unknown user.
- Config `ACCOUNT_DELETION_CANCEL_WINDOW` (default 48h), `ACCOUNT_ERASURE_SWEEP_INTERVAL`, upstream addresses
  `UPSTREAM_ORDER_ADDR`, `UPSTREAM_PAYMENT_ADDR` (required, boot fails without them; no enable flag).
- team-gateway routes the three RPCs for signed-in users only; the internal obligation RPCs stay 501 at the edge
  (owned by `commerce-user-erasure`).

## Impact

- team-identity: `internal/rpc` (new privacy deletion handlers), `internal/identity` (eraser, sweeper),
  `internal/store` (users, sessions, addresses, handoff, outbox, migration), `internal/config`, `cmd` wiring,
  `FEATURES.yaml` (by e2e-author).
- team-gateway: `internal/edge/auth.go` PrivacyService forwarder and route policy (hot file shared with
  `identity-legal-consent` and `identity-data-export`; build serially).
- Depends on `identity-legal-consent` (PrivacyService server registration) and `commerce-user-erasure`
  (obligation RPCs).

## Non-goals

- Consumer-side erasure in other services; export; consent ledger; admin-initiated deletion.
- Blocking new orders while deletion is pending.

## Non-functional requirements

- `RequestAccountDeletion` p95 <= 3 s including both obligation calls (each call single attempt, timeout 2 s);
  `CancelAccountDeletion` and `GetAccountDeletionStatus` p95 <= 100 ms.
- Erasure of one account completes within 72 h of the request (48 h window plus sweep interval <= 1 h, default);
  the identity-side transaction takes <= 2 s p95; the sweeper is idempotent and safe to run on several replicas
  (`FOR UPDATE SKIP LOCKED`).
- Consumers finish within 1 h of `UserErased` locally (initiative NFR, verified by the consumer slices).
- No data loss on crash: erasure and outbox row commit atomically.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `users.deletion_requested_at`, `erase_after` | personal (request metadata) | data-subject request handling | legal obligation | cleared in the erasure transaction; outcome stays as status `deleted` |
| password submitted for re-authentication | credential | re-authentication | contract | never stored or logged; compared against the hash only |
| username, password hash, roles, addresses, sessions, login history (IP, user agent), handoff codes, reset tokens | personal | account operation | contract | erased or anonymised at `erase_after` |
| consent records, seller-terms acceptance | personal (proof) | proof of consent (Decree 13/2023 Art. 9-11) | legal obligation | kept after erasure, N years set by legal (NEEDS_HUMAN, initiative) |
| `UserErased` payload (user id, roles, time) | personal (pseudonymous id) | trigger downstream erasure | legal obligation | outbox retention of the identity outbox |

Logs carry counts and hashed ids only; no raw user id, username, password or address (go-sdk sloglint forbidden keys).

## Threat model

- Assets: the account and its data; the irreversible erasure; the user's sessions.
- Abuse cases: (1) an attacker with a stolen access token deletes the account: mitigated by password re-entry,
  wrong password is `PERMISSION_DENIED` and schedules nothing, and by the 48 h cancel window; (2) user id taken from
  the request to delete someone else's account: the RPCs have no user id field, the id always comes from the
  principal; (3) a seller deletes to dodge pending orders or payouts: blocked by obligations, fail closed when order
  or payment is unreachable; (4) brute-forcing the password through the deletion endpoint: shares the
  `ChangePassword` failed-attempt handling; (5) anonymous or service principal calls the RPCs: `UNAUTHENTICATED` /
  `PERMISSION_DENIED`; (6) replaying the erasure: idempotent, a deleted account is skipped.
- Authz rule: USER principal only, acting on itself only. The sweeper is internal and has no RPC surface.
