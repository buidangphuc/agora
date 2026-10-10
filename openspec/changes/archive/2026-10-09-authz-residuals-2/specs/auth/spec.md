## ADDED Requirements

### Requirement: The admin role is issued the order.admin scope

Tokens issued to an admin account SHALL include `order.admin` in addition to `admin`. Tokens issued to buyer and seller
accounts, and tokens held by service principals, SHALL NOT include `order.admin`.

#### Scenario: An admin token carries order.admin

- **WHEN** the seeded admin logs in through the gateway
- **THEN** the token's scopes include `admin` and `order.admin`

#### Scenario: Buyer and seller tokens carry no order.admin

- **WHEN** a buyer and a seller each log in through the gateway
- **THEN** neither token's scopes include `order.admin` or `admin`
