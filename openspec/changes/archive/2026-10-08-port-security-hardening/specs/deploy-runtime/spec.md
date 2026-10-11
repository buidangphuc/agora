## ADDED Requirements

### Requirement: Services refuse unsafe configuration in staging and production

When `ENV` is staging or production:
- team-payment SHALL refuse to start with `MOCK_PAYMENTS=true`.
- team-identity SHALL refuse to start with the committed development signing key.
- team-order and team-search SHALL refuse to start without durable storage.

Each SHALL exit non-zero with a message naming the offending setting. In local and test environments the same
configuration SHALL be allowed.

#### Scenario: team-payment refuses mock payments in production

- **WHEN** the team-payment image is started with `ENV=production` and `MOCK_PAYMENTS=true`
- **THEN** the process exits non-zero and its log names `MOCK_PAYMENTS`

#### Scenario: team-identity refuses the development signing key in production

- **WHEN** the team-identity image is started with `ENV=production` and the development signing key from the local compose file
- **THEN** the process exits non-zero and its log names the signing key

#### Scenario: team-search refuses in-memory storage in production

- **WHEN** the team-search image is started with `ENV=production` and `DATABASE_ENABLED=false`
- **THEN** the process exits non-zero and its log names the storage setting
