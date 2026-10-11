## MODIFIED Requirements

### Requirement: Internal-only RPCs are not routed

`team-gateway` SHALL NOT route `ListingService/ReserveStock`, `ListingService/ReleaseStock`,
`ListingService/CommitReservation`, `VoucherService/CommitReservation`, `VoucherService/ReleaseReservation` or
`AuditService/WriteAuditEvent`. For every caller, anonymous or authenticated, they SHALL answer `unimplemented`
(HTTP 501) without contacting the upstream service. `VoucherService/ValidateAndReserve` SHALL stay routed.

#### Scenario: Internal stock and voucher saga RPCs answer 501 at the edge

- **WHEN** a logged-in seller calls `ReserveStock`, `ReleaseStock`, `CommitReservation` or `ReleaseReservation` through the gateway
- **THEN** each call answers HTTP 501 with code `unimplemented` and the listing's stock is unchanged

#### Scenario: Writing an audit event is not exposed at the edge

- **WHEN** a logged-in seller or the seeded admin calls `WriteAuditEvent` through the gateway
- **THEN** the gateway answers HTTP 501 and no audit event is stored

#### Scenario: The listing stock commit RPC answers 501 at the edge

- **WHEN** a logged-in seller, then an anonymous caller, calls `ListingService/CommitReservation` through the gateway with
  the reservation id of a buyer's live order
- **THEN** each call answers HTTP 501 with code `unimplemented` and the listing's stock is unchanged
