# identity-account-deletion Specification

## Purpose
TBD - created by archiving change identity-account-deletion. Update Purpose after archive.

## Requirements

### Requirement: Request account deletion with password re-authentication

`PrivacyService.RequestAccountDeletion` SHALL accept a USER principal only, take the user id from the principal,
verify `password` against the stored hash, check open obligations, and then set `deletion_requested_at` to now and
`erase_after` to now plus the cancel window (48 h by default). The system SHALL reject an anonymous caller with
`UNAUTHENTICATED` and a service principal with `PERMISSION_DENIED`.

#### Scenario: Buyer with no obligations schedules deletion

- **WHEN** a signed-in buyer with no open orders, returns, balance or payouts calls `RequestAccountDeletion` with the correct password
- **THEN** the response carries `erase_after` equal to the request time plus 48 h (within 5 s) and `GetAccountDeletionStatus` returns `pending = true` with the same `erase_after`

#### Scenario: Wrong password schedules nothing

- **WHEN** a signed-in user calls `RequestAccountDeletion` with a wrong password
- **THEN** the call fails `PERMISSION_DENIED`, `deletion_requested_at` stays null and no obligation RPC is called

#### Scenario: Empty password is rejected

- **WHEN** a signed-in user calls `RequestAccountDeletion` with an empty password
- **THEN** the call fails `INVALID_ARGUMENT` and nothing is scheduled

#### Scenario: Anonymous caller is unauthenticated

- **WHEN** a caller without a token calls `RequestAccountDeletion` through the gateway
- **THEN** the call fails `UNAUTHENTICATED` and no row changes

#### Scenario: Service principal is denied

- **WHEN** a service principal calls `RequestAccountDeletion` directly on team-identity
- **THEN** the call fails `PERMISSION_DENIED` and no row changes

#### Scenario: Repeated request keeps the first deadline

- **WHEN** a user with deletion already pending calls `RequestAccountDeletion` again with the correct password
- **THEN** the response returns the original `erase_after` unchanged

### Requirement: Open obligations block deletion

The system SHALL call `OrderService.GetUserObligations` and `PaymentService.GetPayoutObligations` with its service
principal before scheduling, and SHALL fail with `FAILED_PRECONDITION` naming the blocker category when
`open_buyer_orders`, `open_seller_orders`, `open_returns`, `available_balance`, `held_balance` or `pending_payouts`
is non-zero. The system SHALL fail closed with `UNAVAILABLE` when either call errors or exceeds 2 s.

#### Scenario: Open order blocks deletion

- **WHEN** a user with `open_buyer_orders = 1` calls `RequestAccountDeletion` with the correct password
- **THEN** the call fails `FAILED_PRECONDITION` with a message naming open orders and `deletion_requested_at` stays null

#### Scenario: Non-zero wallet balance blocks deletion

- **WHEN** a seller with `available_balance = 50000` calls `RequestAccountDeletion` with the correct password
- **THEN** the call fails `FAILED_PRECONDITION` with a message naming the wallet balance and the message contains no amount

#### Scenario: Pending payout blocks deletion

- **WHEN** a seller with `pending_payouts = 1` and zero balances calls `RequestAccountDeletion` with the correct password
- **THEN** the call fails `FAILED_PRECONDITION` naming pending payouts
- **VERIFIED BY**: `team-identity/internal/rpc` `TestRequestAccountDeletionPendingPayoutBlocks`

#### Scenario: Obligations are checked for the caller only

- **WHEN** user A with no obligations calls `RequestAccountDeletion` while user B has an open order
- **THEN** the obligation calls carry A's id from the principal, never B's, and the request succeeds

#### Scenario: Zero obligations on every counter allows deletion

- **WHEN** both obligation calls return all zeros for the user
- **THEN** the request succeeds and `erase_after` is set

#### Scenario: Payment service unreachable fails closed

- **WHEN** `GetPayoutObligations` times out after 2 s
- **THEN** the call fails `UNAVAILABLE` and `deletion_requested_at` stays null
- **VERIFIED BY**: `team-identity/internal/rpc` `TestRequestAccountDeletionPaymentTimeoutFailsClosed`

#### Scenario: Order service unreachable fails closed

- **WHEN** `GetUserObligations` returns an error
- **THEN** the call fails `UNAVAILABLE`, `GetPayoutObligations` result is not used and nothing is scheduled
- **VERIFIED BY**: `team-identity/internal/rpc` `TestRequestAccountDeletionOrderErrorFailsClosed`

### Requirement: Scheduling revokes other sessions

When deletion is scheduled the system SHALL revoke every session of the user except the session of the calling
access token, in the same transaction as setting the timestamps.

#### Scenario: Other sessions are revoked, current stays

- **WHEN** a user with three active sessions schedules deletion from session A
- **THEN** sessions B and C are revoked, session A stays active, and refreshing with B's refresh token fails `UNAUTHENTICATED`

#### Scenario: Failed scheduling revokes nothing

- **WHEN** scheduling fails with `FAILED_PRECONDITION` because of an open order
- **THEN** all three sessions stay active

### Requirement: Cancel deletion before erase_after

`PrivacyService.CancelAccountDeletion` SHALL clear `deletion_requested_at` and `erase_after` for the calling user
while deletion is pending and no erasure has happened.

#### Scenario: Cancel within the window

- **WHEN** a user with pending deletion calls `CancelAccountDeletion`
- **THEN** `GetAccountDeletionStatus` returns `pending = false` and the sweeper does not erase the account at the former `erase_after`

#### Scenario: Cancel with nothing pending is rejected

- **WHEN** a user with no pending deletion calls `CancelAccountDeletion`
- **THEN** the call fails `FAILED_PRECONDITION`

#### Scenario: Anonymous cancel is unauthenticated

- **WHEN** a caller without a token calls `CancelAccountDeletion`
- **THEN** the call fails `UNAUTHENTICATED`

#### Scenario: A user cannot cancel another user's deletion

- **WHEN** user B calls `CancelAccountDeletion` while only user A has deletion pending
- **THEN** the call fails `FAILED_PRECONDITION` and user A's `erase_after` is unchanged

### Requirement: Deletion status reflects the caller's own state

`PrivacyService.GetAccountDeletionStatus` SHALL return `pending` and `erase_after` for the calling user only.

#### Scenario: Status with nothing pending

- **WHEN** a user who never requested deletion calls `GetAccountDeletionStatus`
- **THEN** the response has `pending = false` and no `erase_after`

#### Scenario: Status with pending deletion

- **WHEN** a user with pending deletion calls `GetAccountDeletionStatus`
- **THEN** the response has `pending = true` and the stored `erase_after`

#### Scenario: Anonymous status is unauthenticated

- **WHEN** a caller without a token calls `GetAccountDeletionStatus`
- **THEN** the call fails `UNAUTHENTICATED`

### Requirement: Sweeper erases due accounts and publishes UserErased

The erasure sweeper SHALL, for every account with `erase_after` at or before now and status other than `deleted`,
run one transaction that anonymises the username to `deleted_<random>`, clears the password hash and roles, deletes
the user's addresses, sessions, login history, handoff codes and password-reset tokens, keeps consent and
seller-terms records, sets status `deleted`, clears the deletion timestamps and inserts one `UserErased` outbox
row (key = user id, `roles` = roles held before erasure, `erased_at` = now).

#### Scenario: Due account is erased

- **WHEN** the sweeper runs for a user whose `erase_after` is in the past
- **THEN** the user row has status `deleted`, a username matching `deleted_[0-9a-f]{16}`, an empty password hash, and zero rows remain in addresses, sessions, login history, handoff codes and reset tokens for that id

#### Scenario: UserErased is written in the same transaction

- **WHEN** an account is erased
- **THEN** exactly one outbox row of type `platform.identity.v1.UserErased` exists for the user id with the roles held before erasure, and if the transaction rolls back neither the row nor any deletion remains

#### Scenario: Consent and seller-terms proof survive erasure

- **WHEN** an account with two consent records and a seller-terms acceptance is erased
- **THEN** the same two consent records and the seller-terms acceptance still exist after erasure

#### Scenario: Account not yet due is untouched

- **WHEN** the sweeper runs at `erase_after` minus 1 second
- **THEN** the user row, sessions and addresses are unchanged and no `UserErased` row is written

#### Scenario: Cancelled account is untouched

- **WHEN** the sweeper runs after the user cancelled and `erase_after` is null
- **THEN** the account is not erased

#### Scenario: Sweep is idempotent

- **WHEN** the sweeper runs twice over the same due account, or two replicas run concurrently
- **THEN** exactly one `UserErased` outbox row exists for that user id

#### Scenario: Erasure completes within 72 h of the request

- **WHEN** a user requests deletion at T and the sweeper interval is 1 h
- **THEN** the account is erased no later than T plus 49 h and `UserErased` is published to `identity.events` within that bound

#### Scenario: One failing account does not stop the batch

- **WHEN** erasing one due account fails with a database error
- **THEN** that account is left pending for the next sweep and the other due accounts in the batch are erased
- **VERIFIED BY**: `team-identity/internal/identity` `TestEraserOneFailingAccountDoesNotStopBatch`

### Requirement: Deleted accounts cannot sign in

`Login`, `Refresh` and handoff exchange SHALL refuse an account with status `deleted` exactly like an unknown
user, and scopes for status `deleted` SHALL be empty.

#### Scenario: Login to an erased account fails

- **WHEN** someone calls `Login` with the former username and former password of an erased account
- **THEN** the call fails with the same code and message as an unknown username

#### Scenario: Former username is not reusable by lookup

- **WHEN** a visitor registers the former username of an erased account
- **THEN** Register succeeds because the stored username is `deleted_<random>`

#### Scenario: Pending deletion does not block sign-in

- **WHEN** a user with pending deletion calls `Login` with the correct password
- **THEN** Login succeeds so the user can cancel

### Requirement: Gateway routes the lifecycle RPCs for signed-in users

team-gateway SHALL forward `RequestAccountDeletion`, `CancelAccountDeletion` and `GetAccountDeletionStatus` to
team-identity for signed-in users and SHALL answer 501 for `OrderService.GetUserObligations` and
`PaymentService.GetPayoutObligations`.

#### Scenario: Signed-in user reaches the RPC

- **WHEN** a signed-in user posts `GetAccountDeletionStatus` through the gateway
- **THEN** the gateway returns 200 with the status body

#### Scenario: Anonymous request is rejected at the edge

- **WHEN** a request without a token posts `RequestAccountDeletion` through the gateway
- **THEN** the gateway returns 401 and team-identity receives no call

#### Scenario: Internal obligation RPCs are not exposed

- **WHEN** any caller posts `GetUserObligations` or `GetPayoutObligations` through the gateway
- **THEN** the gateway returns 501
- **VERIFIED BY**: `team-gateway/internal/edge` `TestObligationRPCsAreNotRoutedAtTheGateway`

### Requirement: Deletion logs carry no personal data

The system SHALL NOT log raw user ids, usernames, passwords or addresses in the deletion and erasure paths.

#### Scenario: Erasure logs counts only

- **WHEN** the sweeper erases an account
- **THEN** captured log output contains no raw user id, former username or password and does contain the erased count
- **VERIFIED BY**: `team-identity/internal/identity` `TestErasureLogsCountsOnly`

#### Scenario: A failed erasure logs no personal data

- **WHEN** erasing an account fails with a database error
- **THEN** the error log line contains no raw user id and no former username
- **VERIFIED BY**: `team-identity/internal/identity` `TestFailedErasureLogsNoPersonalData`
