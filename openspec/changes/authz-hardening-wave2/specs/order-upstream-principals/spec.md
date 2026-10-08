## Purpose

Defines the least-privilege principals `team-order` presents when it calls `team-domain` and `team-identity`, so a forwarded
buyer is not silently upgraded and a background call never carries write scopes it does not need.

## ADDED Requirements

### Requirement: Unmarked upstream calls carry no more than listing.read

A `team-order` upstream call that is not explicitly marked as a service call SHALL present either the forwarded end-user principal
unchanged, or, when there is no incoming principal, the service principal `service-team-order` with the single scope
`listing.read`. `team-order` SHALL NOT append scopes to a forwarded principal and SHALL NOT present `listing.write`,
`identity.read`, `identity.write` or `inventory.write` on an unmarked call.

#### Scenario: A forwarded buyer is sent unchanged

- **WHEN** `team-order` reads a listing for the cart inside a buyer's request
- **THEN** `team-domain` receives exactly the buyer's forwarded principal id, type and scopes

#### Scenario: A background unmarked call has read-only scope

- **WHEN** an unmarked upstream call runs with no incoming principal
- **THEN** it carries `service-team-order` with only `listing.read`

#### Scenario: Stock and promotion calls keep their single service scope

- **WHEN** the order saga reserves, commits or releases stock, or reserves, commits or releases a voucher
- **THEN** each call carries `service-team-order` with exactly `inventory.write` or exactly `promotion.reserve` respectively, never both

### Requirement: Raw upstream clients are not exported

`team-order` SHALL NOT expose an unwrapped upstream listing client to its service code; every listing call SHALL go through the
wrapped domain client so the stock RPCs cannot be issued with a forwarded buyer by mistake.

#### Scenario: Stock RPCs cannot bypass the marker

- **WHEN** code in the service layer issues a stock RPC through the only available domain client
- **THEN** the call is marked as a service call with `inventory.write`
