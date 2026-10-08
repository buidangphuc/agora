## Purpose

Defines who may settle, read, refund and withdraw money in `team-payment`, so that payment settlement, seller wallets and payouts
cannot be driven by anonymous or unrelated signed-in callers and the mock settlement path cannot exist in a deployed environment.

## ADDED Requirements

### Requirement: Mock settlement requires the paying buyer and an explicit opt-in

`team-payment` SHALL require an authenticated principal on `ProcessMockPayment` and SHALL settle or fail a transaction only
when the principal is the transaction's buyer; any other caller, and an unknown transaction id, SHALL receive `NOT_FOUND` and
the transaction SHALL be unchanged. The RPC SHALL run only when mock payments are enabled (`MOCK_PAYMENTS=true`); when disabled
it SHALL return `FAILED_PRECONDITION` without reading the transaction. The service SHALL refuse to start when mock payments
are enabled and `ENV` is staging, stage, prod or production. A caller without a principal or with the anonymous principal
SHALL receive `UNAUTHENTICATED`.

#### Scenario: Anonymous caller cannot settle a payment

- **WHEN** an anonymous caller calls `ProcessMockPayment` with an existing pending transaction id and `simulate_success=true`
- **THEN** the call is `UNAUTHENTICATED`, the transaction stays pending, no `PaymentSettled` event is emitted and no wallet is credited

#### Scenario: Another buyer cannot settle someone else's payment

- **WHEN** buyer B calls `ProcessMockPayment` for a transaction whose buyer is A
- **THEN** the call is `NOT_FOUND` and the transaction is unchanged

#### Scenario: The paying buyer settles their own payment

- **WHEN** buyer A calls `ProcessMockPayment` for their own pending transaction with mock payments enabled
- **THEN** the transaction becomes paid, `PaymentSettled` is emitted once and the order's seller wallet is credited

#### Scenario: Mock settlement is refused when disabled

- **WHEN** `MOCK_PAYMENTS` is false and the owning buyer calls `ProcessMockPayment`
- **THEN** the call is `FAILED_PRECONDITION` and the transaction is unchanged

#### Scenario: Mock payments cannot be enabled in a deployed environment

- **WHEN** the service starts with `MOCK_PAYMENTS=true` and `ENV` set to `staging`, `stage`, `prod` or `production`
- **THEN** startup fails with an error naming `MOCK_PAYMENTS` and the environment

### Requirement: Wallet and payout RPCs are bound to the caller's seller identity

For `GetSellerWallet`, `ListPayoutHistory`, `RequestPayout`, `GetWalletBalance`, `ListLedgerEntries` and
`RequestWalletPayout`, `team-payment` SHALL treat the authenticated principal's id as the seller id. A request `seller_id`
that is empty or equals the principal id SHALL be accepted; any other `seller_id` SHALL be accepted only when the principal
holds the scope `admin`, and otherwise SHALL be `PERMISSION_DENIED` with no data returned and no state changed.
`RequestPayout` and `RequestWalletPayout` SHALL additionally require the scope `listing.write`. A caller without a principal
or with the anonymous principal SHALL receive `UNAUTHENTICATED`.

#### Scenario: A buyer cannot read another seller's wallet

- **WHEN** a signed-in buyer calls `GetWalletBalance` with `seller_id` set to seller S
- **THEN** the call is `PERMISSION_DENIED` and no balance is returned

#### Scenario: A buyer cannot drain another seller's wallet

- **WHEN** a signed-in buyer calls `RequestWalletPayout` or `RequestPayout` with `seller_id` set to seller S, a valid amount and their own bank account
- **THEN** the call is `PERMISSION_DENIED` and S's balance and ledger are unchanged

#### Scenario: A buyer without the seller scope cannot request a payout for themselves

- **WHEN** a buyer (no `listing.write`) calls `RequestWalletPayout` with an empty `seller_id`
- **THEN** the call is `PERMISSION_DENIED` and no ledger entry is written

#### Scenario: A seller reads and withdraws from their own wallet

- **WHEN** a seller calls `GetWalletBalance`, `ListLedgerEntries` and `RequestWalletPayout` with an empty or own `seller_id` and an amount within the balance
- **THEN** the balance and ledger are their own, and the payout is recorded once for their id

#### Scenario: Admin may read any seller's wallet

- **WHEN** the seeded admin calls `GetSellerWallet` with `seller_id` set to seller S
- **THEN** the call succeeds and returns S's wallet

#### Scenario: Anonymous callers are rejected on every wallet RPC

- **WHEN** an anonymous caller calls any of the six wallet and payout RPCs
- **THEN** each call is `UNAUTHENTICATED`

### Requirement: Refunds are limited to admin and the order's seller

`team-payment` SHALL allow `RefundPayment` only for a principal holding `admin`, or for the seller of the order the payment
belongs to (resolved from `team-order` as the service principal `service-team-payment` with scope `order.read`). Every other
caller, an unknown payment, and an unknown order SHALL receive `NOT_FOUND` without revealing whether the payment exists; when
the order lookup fails or is unconfigured and the caller is not admin the call SHALL fail closed with `UNAVAILABLE`. The
payment SHALL NOT change on any rejection. Existing state rules (only a paid payment, amount not above the transaction) SHALL be unchanged.

#### Scenario: A buyer cannot refund a paid payment

- **WHEN** the buyer of a paid payment, then an unrelated buyer, call `RefundPayment` with the payment id and with the order id
- **THEN** every call is `NOT_FOUND` and the payment stays paid

#### Scenario: The order's seller refunds

- **WHEN** the seller of the order calls `RefundPayment` for the paid payment with a valid amount
- **THEN** the payment becomes refunded

#### Scenario: Admin refunds without a lookup

- **WHEN** the seeded admin calls `RefundPayment` while `team-order` is unreachable
- **THEN** the refund is applied

#### Scenario: Refund fails closed when the order cannot be read

- **WHEN** a seller calls `RefundPayment` and the order lookup fails
- **THEN** the call is `UNAVAILABLE` and the payment is unchanged

### Requirement: Transactions are readable only by their parties

`team-payment` SHALL require an authenticated principal on `GetPayment` and SHALL return a transaction only to its buyer, to
the seller of its order, or to `admin`; every other caller and unknown ids SHALL receive `NOT_FOUND` with no transaction data. A
caller without a principal or with the anonymous principal SHALL receive `UNAUTHENTICATED`.

#### Scenario: Anonymous caller cannot read a transaction by order id

- **WHEN** an anonymous caller calls `GetPayment` with an order id
- **THEN** the call is `UNAUTHENTICATED` and no transaction is returned

#### Scenario: A stranger cannot read a transaction

- **WHEN** a signed-in user who is neither the buyer, the order's seller nor admin calls `GetPayment` for that order id
- **THEN** the call is `NOT_FOUND`, identical to an unknown order id

#### Scenario: The buyer reads their own transaction by order id

- **WHEN** the order's buyer calls `GetPayment` with the order id (the checkout payment page)
- **THEN** the transaction is returned
