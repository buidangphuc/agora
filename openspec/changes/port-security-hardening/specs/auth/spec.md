## ADDED Requirements

### Requirement: Role tokens carry the AI and recommendation scopes

Tokens issued to buyer, seller and admin accounts SHALL include `recommendations:read` and `ai:use`. Service-only
scopes (`inventory.write`, `order.read`, `promotion.reserve`, `audit.write`, `features.read`, `features.dataset`)
SHALL NOT be granted to any role.

#### Scenario: A buyer token carries the AI scopes and no service scope

- **WHEN** a buyer logs in through the gateway
- **THEN** the token's scopes include `recommendations:read` and `ai:use` and include none of the service-only scopes

### Requirement: Login attempts are recorded

A successful login SHALL add a success entry to the user's login history. A wrong password for an existing user SHALL
add a failure entry. An unknown username SHALL record nothing and SHALL return the same error as a wrong password.

#### Scenario: Login history shows a failed and a successful attempt

- **WHEN** a user logs in once with a wrong password and then with the right one, and lists their login history through the gateway
- **THEN** the history contains a failure entry followed by a success entry
