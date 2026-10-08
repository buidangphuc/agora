## Purpose

Defines who may read seller funnel and revenue analytics from `team-analytics`, so one seller's business metrics are not readable
by anonymous callers or other users.

## ADDED Requirements

### Requirement: Seller analytics are readable only by the seller or admin

`team-analytics` SHALL resolve the forwarded principal and SHALL require the scope `listing.write` on `GetSellerFunnel` and
`GetRevenueBreakdown`. The effective seller id SHALL be the principal id; a request `seller_id` that is empty or equals the
principal id SHALL be accepted, and any other `seller_id` SHALL be accepted only for a principal holding `admin`, otherwise
`PERMISSION_DENIED` with no data returned. A caller without a principal or with the anonymous principal SHALL receive
`UNAUTHENTICATED`. The warehouse queries SHALL be unchanged.

#### Scenario: Anonymous caller cannot read analytics

- **WHEN** an anonymous caller calls `GetSellerFunnel` and `GetRevenueBreakdown` with any `seller_id`
- **THEN** both calls are `UNAUTHENTICATED` and no data is returned

#### Scenario: A buyer cannot read analytics

- **WHEN** a signed-in buyer (no `listing.write`) calls `GetSellerFunnel`
- **THEN** the call is `PERMISSION_DENIED`

#### Scenario: A seller cannot read another seller's analytics

- **WHEN** seller A calls `GetRevenueBreakdown` with `seller_id` set to seller B
- **THEN** the call is `PERMISSION_DENIED` and no revenue is returned

#### Scenario: A seller reads their own analytics

- **WHEN** a seller calls `GetSellerFunnel` and `GetRevenueBreakdown` with their own id or an empty `seller_id`
- **THEN** the funnel and revenue for their own id are returned

#### Scenario: Admin reads any seller's analytics

- **WHEN** the seeded admin calls `GetSellerFunnel` with `seller_id` set to a seller
- **THEN** that seller's funnel is returned
