## ADDED Requirements

### Requirement: The admin role is issued the promotion.admin scope

Tokens issued to an admin account SHALL include `promotion.admin` in addition to `admin` and `order.admin`. Tokens
issued to buyer and seller accounts, and tokens held by service principals, SHALL NOT include `promotion.admin`.

#### Scenario: An admin token carries promotion.admin

- **WHEN** the seeded admin logs in through the gateway
- **THEN** the token's scopes include `admin` and `promotion.admin`

#### Scenario: Buyer and seller tokens never carry promotion.admin

- **WHEN** a buyer and a seller each log in through the gateway
- **THEN** neither token carries the `promotion.admin` scope or the `admin` scope
