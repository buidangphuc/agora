## Purpose

Defines who may preview, hold, commit and release voucher reservations, subscribe to seller plans, create sponsored
ad campaigns and read plan entitlements.

## ADDED Requirements

### Requirement: Voucher saga RPCs are service-only

`CommitReservation` and `ReleaseReservation` SHALL require a SERVICE principal holding `promotion.reserve`.
`ValidateAndReserve` from such a service SHALL be trusted as-is. From any other authenticated caller it SHALL be a
preview: `buyer_id` SHALL be forced to the caller's own id, and `reservation_id` SHALL start with `preview:<caller
id>:`, otherwise the call SHALL fail with `permission_denied`. An anonymous caller SHALL receive `unauthenticated`.

#### Scenario: A buyer previews a voucher under their own namespace

- **WHEN** a logged-in buyer calls `ValidateAndReserve` through the gateway with `reservation_id` `preview:<own id>:<code>`
- **THEN** the call succeeds and the discount is computed for that buyer

#### Scenario: A buyer cannot hold a reservation outside the preview namespace

- **WHEN** a logged-in buyer calls `ValidateAndReserve` through the gateway with a `reservation_id` that does not start with `preview:<own id>:`
- **THEN** the gateway answers HTTP 403

#### Scenario: A voucher checkout still completes through the saga

- **WHEN** a buyer places an order with a valid voucher and pays
- **THEN** the order is paid with the voucher discount applied and the voucher redemption is committed

### Requirement: Plan subscription and sponsored ads need seller rights

`Subscribe` and `CreateAdCampaign` SHALL require `listing.write`. `CreateAdCampaign` SHALL additionally require that
the caller owns the listing, unless the caller is an admin. A bid above `MAX_AD_BID` or a budget above
`MAX_AD_BUDGET` SHALL be rejected with `invalid_argument`. `GetEntitlements` SHALL return a seller's plan
entitlements only to that seller or to a SERVICE principal; any other caller SHALL receive `permission_denied`.

#### Scenario: A buyer cannot subscribe to a seller plan

- **WHEN** a logged-in buyer calls `Subscribe` through the gateway
- **THEN** the gateway answers HTTP 403

#### Scenario: A seller cannot advertise another seller's listing

- **WHEN** a logged-in seller calls `CreateAdCampaign` through the gateway for a listing owned by another seller
- **THEN** the gateway answers HTTP 403

#### Scenario: An oversized ad bid is rejected

- **WHEN** a logged-in seller calls `CreateAdCampaign` for their own listing with a bid above the configured maximum
- **THEN** the gateway answers HTTP 400

#### Scenario: A seller cannot read another seller's entitlements

- **WHEN** a logged-in seller calls `GetEntitlements` through the gateway for another seller's id
- **THEN** the gateway answers HTTP 403
