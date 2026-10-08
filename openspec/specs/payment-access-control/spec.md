# payment-access-control Specification

## Purpose
Defines when the mock payment path is available, so that it can never settle real orders outside local and test
environments.

## Requirements

### Requirement: Mock payments are opt-in

`ProcessMockPayment` SHALL be refused with `failed_precondition` unless team-payment runs with `MOCK_PAYMENTS=true`.
The local compose stack enables it.

#### Scenario: The local stack settles a checkout through the mock payment

- **WHEN** a buyer places an order on the local stack and pays with the mock payment
- **THEN** the payment succeeds and the order becomes paid
