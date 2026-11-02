# payment-access-control Specification

## Purpose
Defines when the mock payment path is available, so that it can never settle real orders outside local and test
environments.

## Requirements

### Requirement: Mock payments are opt-in

`ProcessMockPayment` SHALL be refused with `failed_precondition` unless team-payment runs with `MOCK_DATA` naming the `payment-provider` adapter (see `payment-service-runtime`). The local compose stack enables it. `MOCK_PAYMENTS` SHALL NOT enable it.

#### Scenario: The local stack settles a checkout through the mock payment

- **WHEN** a buyer places an order on the local stack and pays with the mock payment
- **THEN** the payment succeeds and the order becomes paid
- **VERIFIED BY**: platform-e2e `features/payment/mock_pay.feature` `Buyer completes mock payment for an order`; team-payment `internal/rpc` `TestMockDataProviderSettlesThroughRealPath`

#### Scenario: A stack without the adapter refuses the mock payment

- **WHEN** team-payment runs with `MOCK_DATA` empty and a buyer calls `ProcessMockPayment`
- **THEN** the call fails with `failed_precondition` and the order stays pending
- **VERIFIED BY**: team-payment `internal/rpc` `TestProcessMockPayment_DisabledByDefault`

### Requirement: Payouts require the seller scope

`RequestPayout` and `RequestWalletPayout` SHALL require the caller to hold `listing.write`, in addition to acting on
their own wallet. A caller without it SHALL get `PERMISSION_DENIED` and no payout SHALL be recorded.

#### Scenario: A buyer cannot request a payout

- **WHEN** a logged-in buyer, who holds no `listing.write`, calls `RequestWalletPayout` for their own id through the
  gateway
- **THEN** the call fails with `permission_denied` and the buyer's wallet ledger has no `PAYOUT` entry

#### Scenario: A seller can still request a payout

- **WHEN** a seller with an available balance calls `RequestWalletPayout` for part of it through the gateway
- **THEN** a `PAYOUT` entry for that amount appears in the seller's wallet ledger
