## Purpose

Defines who may create sponsored ad campaigns, subscribe to seller plans and read plan entitlements in `team-promotion`, so
placement ranking and paid-tier state cannot be taken by unrelated or unauthenticated callers.

## ADDED Requirements

### Requirement: Ad campaigns require listing ownership and bounded amounts

`CreateAdCampaign` SHALL require the scope `listing.write` and SHALL accept a `listing_id` only when the principal owns that
listing (as reported by `team-domain` for the service principal) or holds `admin` (no lookup). A listing that does not exist or
belongs to another seller SHALL be `PERMISSION_DENIED`, indistinguishable from each other; an unreachable or unconfigured
ownership lookup for a non-admin SHALL be `UNAVAILABLE`; no campaign SHALL be created on any rejection. `budget` and `bid`
SHALL each be non-negative and not above configured maxima (`MAX_AD_BUDGET`, `MAX_AD_BID`), otherwise `INVALID_ARGUMENT`. The
stored `seller_id` SHALL be the principal id.

#### Scenario: A buyer cannot create an ad campaign

- **WHEN** a buyer (no `listing.write`) calls `CreateAdCampaign`
- **THEN** the call is `PERMISSION_DENIED` and no campaign exists

#### Scenario: A seller cannot promote another seller's listing

- **WHEN** seller A calls `CreateAdCampaign` for a listing owned by seller B
- **THEN** the call is `PERMISSION_DENIED` and no campaign exists

#### Scenario: A seller promotes their own listing

- **WHEN** seller A calls `CreateAdCampaign` for their own listing with a bid and budget within the maxima
- **THEN** an active campaign is stored with `seller_id` equal to A

#### Scenario: An oversized bid is rejected

- **WHEN** a seller calls `CreateAdCampaign` with a `bid` above `MAX_AD_BID`
- **THEN** the call is `INVALID_ARGUMENT` and no campaign exists

#### Scenario: Ownership lookup failure fails closed for non-admin

- **WHEN** a seller calls `CreateAdCampaign` while `team-domain` is unreachable
- **THEN** the call is `UNAVAILABLE` and no campaign exists, while the admin path still succeeds

### Requirement: Plan subscription is a seller action

`Subscribe` SHALL require the scope `listing.write` (seller, admin) in addition to a principal, and SHALL bind the subscriber to
the principal id. A caller holding only buyer scopes SHALL be `PERMISSION_DENIED` and no subscription SHALL be stored.

#### Scenario: A buyer cannot take a paid plan

- **WHEN** a buyer calls `Subscribe` for the PREMIUM plan
- **THEN** the call is `PERMISSION_DENIED` and their entitlements remain FREE

#### Scenario: A seller subscribes

- **WHEN** a seller calls `Subscribe` for a plan
- **THEN** a subscription for the seller's id is stored and `GetEntitlements` reflects the plan

### Requirement: Entitlement reads are isolated for every non-service principal

`GetEntitlements` SHALL let a principal read only its own entitlements unless its type is `service`, in which case any seller may
be read. Every non-service principal type (user, anonymous, unknown) asking for another seller's id SHALL receive
`PERMISSION_DENIED`; the anonymous principal SHALL be `UNAUTHENTICATED`.

#### Scenario: A user cannot read another seller's entitlements

- **WHEN** seller A calls `GetEntitlements` with `seller_id` set to seller B
- **THEN** the call is `PERMISSION_DENIED`

#### Scenario: A principal of unknown type cannot read another seller

- **WHEN** a principal with an unspecified type calls `GetEntitlements` for another seller
- **THEN** the call is `PERMISSION_DENIED`

#### Scenario: A service principal reads any seller

- **WHEN** a principal of type `service` calls `GetEntitlements` for a seller
- **THEN** that seller's entitlements are returned
