## MODIFIED Requirements

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
