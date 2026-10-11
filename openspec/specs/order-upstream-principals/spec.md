# order-upstream-principals Specification

## Purpose
Defines the identity team-order presents to the services it calls, and the rule that a buyer cannot purchase their
own listing.

## Requirements

### Requirement: team-order calls upstream with least privilege

team-order SHALL call the voucher saga RPCs as the SERVICE principal `service-team-order` with exactly the scope
`promotion.reserve`, never forwarding the buyer's principal or bearer token. It SHALL call `ReserveStock`,
`CommitReservation` and `ReleaseStock` of `ListingService` as that service principal with exactly `listing.write`,
whether the call is made inside a buyer's request or by background compensation and sweeping. It SHALL forward the
buyer's own scopes unchanged on reads made for the buyer, without adding scopes.

#### Scenario: A buyer checkout reserves and commits stock and voucher

- **WHEN** a buyer checks out a cart with a voucher and the payment settles
- **THEN** the listing's stock decreases by the ordered quantity and the voucher is redeemed once

#### Scenario: A buyer checkout commits stock as the order service

- **WHEN** a buyer, whose own scopes do not include `listing.write`, checks out a cart and the configured reservation TTL
  plus two sweep intervals then pass
- **THEN** the order is created and the listing's stock stays reduced by the ordered quantity

### Requirement: A buyer cannot buy their own listing

team-order SHALL reject an order whose buyer is the seller of a listing in it, with `failed_precondition`, before
any stock or voucher is reserved.

#### Scenario: A seller cannot check out their own listing

- **WHEN** a seller adds their own listing to their cart and places the order through the gateway
- **THEN** the order is refused and the listing's stock is unchanged
