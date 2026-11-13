## ADDED Requirements

### Requirement: GetPayoutObligations reports a user's wallet money and pending payouts

`PaymentService.GetPayoutObligations` SHALL return, for the requested `user_id` as seller, `available_balance` (the
ledger balance minus the held amount, never below 0), `held_balance` (proceeds still inside the refund hold window)
and `pending_payouts` (payout requests in status PENDING or PROCESSING). It SHALL accept only a SERVICE principal
with scope `payment.internal.read` and SHALL NOT be routed at the gateway.

#### Scenario: PAY-1 Balance, hold and pending payouts are reported
- **GIVEN** seller S has ledger credits of 500000, of which 200000 are inside the hold window, and 2 PENDING payout debits of 50000 each
- **WHEN** a SERVICE principal with scope `payment.internal.read` calls `GetPayoutObligations` for S
- **THEN** the response is `available_balance` 200000, `held_balance` 200000 and `pending_payouts` 2

#### Scenario: PAY-2 A buyer with no wallet reads zeros
- **WHEN** the service calls `GetPayoutObligations` for a user with no ledger entries and no payout requests
- **THEN** the response is OK with `available_balance` 0, `held_balance` 0, `pending_payouts` 0

#### Scenario: PAY-3 Completed and rejected payouts are not pending
- **GIVEN** S has payout requests in COMPLETED and REJECTED status only
- **WHEN** the service calls `GetPayoutObligations` for S
- **THEN** `pending_payouts` is 0

#### Scenario: PAY-4 A negative balance reads as no available money
- **GIVEN** S's ledger sum is -50000 after a refund deduction
- **WHEN** the service calls `GetPayoutObligations` for S
- **THEN** `available_balance` is 0

#### Scenario: PAY-5 Another seller's money is not included
- **GIVEN** S has ledger credits of 100000 outside the hold window and T has ledger credits of 900000
- **WHEN** the service calls `GetPayoutObligations` for S
- **THEN** `available_balance` is 100000 and `held_balance` is 0

#### Scenario: PAY-6 An empty user id is rejected
- **WHEN** the service calls `GetPayoutObligations` with `user_id` ""
- **THEN** the call fails with `INVALID_ARGUMENT`

#### Scenario: PAY-7 A user principal is refused
- **WHEN** a signed-in USER principal, including a seller reading its own wallet, calls `GetPayoutObligations`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: PAY-8 A service principal without the scope is refused
- **WHEN** a SERVICE principal holding only `order.read` calls `GetPayoutObligations`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: PAY-9 An anonymous caller is refused
- **WHEN** a call without a principal reaches `GetPayoutObligations`
- **THEN** the call fails with `UNAUTHENTICATED`

#### Scenario: PAY-10 The gateway does not route the RPC
- **WHEN** a browser client calls `PaymentService/GetPayoutObligations` through the gateway
- **THEN** the gateway answers HTTP 501 and team-payment receives no request

#### Scenario: PAY-11 A store failure never reads as zero money
- **GIVEN** the payment database is unreachable
- **WHEN** the service calls `GetPayoutObligations` for S
- **THEN** the call fails with a non-OK gRPC status and the response carries no balances
