## ADDED Requirements

### Requirement: Service-only scopes are declared and granted to no user role

`team-identity` SHALL NOT grant any of the service-only scopes `inventory.write`, `order.read`, `promotion.reserve` or
`audit.write` to any user role (buyer, seller, admin), and the authorization tests SHALL declare every service-only scope
explicitly, with the owning service and the holder, and fail when any role is granted one. Adding a scope that a service
enforces SHALL require declaring it either as a user scope granted to some role or as a service-only scope in that list.

#### Scenario: No role holds a service-only scope

- **WHEN** the scopes of every role (buyer, seller, admin) are enumerated
- **THEN** none contains `inventory.write`, `order.read`, `promotion.reserve` or `audit.write`

#### Scenario: A user token never carries a service-only scope

- **WHEN** a buyer, a seller and the seeded admin log in
- **THEN** none of the issued tokens carries any service-only scope

#### Scenario: An undeclared enforced scope fails the drift test

- **WHEN** a scope enforced by a service is neither granted to a role nor declared service-only
- **THEN** the authorization coverage test fails naming that scope and its service

### Requirement: Admin-only RPCs are gated by the admin scope

`team-identity` SHALL grant the scope `admin` to the admin role only, and services and the gateway SHALL use that scope
(not `engagement:write`, `engagement:read` or any scope shared by buyer and seller) to gate admin-only RPCs
(`ResolveDispute`, `ReviewKyc`, `QueryAuditLog`, `CreateVoucher` with scope `PLATFORM`). The admin scope SHALL NOT be
obtainable through registration.

#### Scenario: Only the admin role holds the admin scope

- **WHEN** the scopes of buyer, seller and admin are enumerated
- **THEN** only admin contains `admin`

#### Scenario: Registering as admin yields a buyer

- **WHEN** an account is registered with role `admin` and logs in
- **THEN** the issued token's scopes are the buyer's and do not include `admin`
