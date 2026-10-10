## ADDED Requirements

### Requirement: Only a user can place an order

`CreateOrder` SHALL require a USER principal. A SERVICE principal or any other non-user principal SHALL get
`PERMISSION_DENIED`, whatever its scopes, and SHALL NOT reserve stock or create an order. No service places orders on a
buyer's behalf.

#### Scenario: A service token cannot place an order

- **WHEN** a client calls `CreateOrder` through the gateway with a validly signed SERVICE token for a cart line of a
  listing with stock 5
- **THEN** the call fails with `permission_denied`, no order is created, and the listing's stock is still 5
