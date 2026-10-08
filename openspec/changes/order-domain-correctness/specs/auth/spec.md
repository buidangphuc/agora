## ADDED Requirements

### Requirement: Identity grants every scope that services enforce for user roles

`team-identity` SHALL grant each scope that a service enforces on behalf of a signed-in user to the roles intended to
use it, so that a legitimate call is never rejected with `insufficient_scope` only because identity never issued the
scope. In particular `recommendations:read` (enforced by `team-ai` on `Recommend`) SHALL be granted to buyer, seller
and admin. A scope reserved for service-to-service calls (`inventory.write`) SHALL NOT be granted to any user role.
The scope table SHALL be covered by a test that lists the scopes services enforce and fails when one is granted to no
intended role.

#### Scenario: A buyer token carries recommendations:read

- **WHEN** a buyer logs in
- **THEN** the issued token's scopes include `recommendations:read`

#### Scenario: Seller and admin tokens carry recommendations:read

- **WHEN** a seller, then an admin, log in
- **THEN** each issued token's scopes include `recommendations:read`

#### Scenario: A logged-in buyer is not rejected by the recommendation scope gate

- **WHEN** a logged-in buyer calls `Recommend` through the gateway
- **THEN** the response is not `PERMISSION_DENIED` with `insufficient_scope` (it is either recommendations or the service's "not enabled" answer)

#### Scenario: The service-only stock scope is granted to no role

- **WHEN** the scopes of every role (buyer, seller, admin) are enumerated
- **THEN** none contains `inventory.write`

#### Scenario: An enforced scope that no role receives fails the test

- **WHEN** the enforced-scope list contains a scope that no intended role is granted
- **THEN** the scope-coverage test fails and names the scope
