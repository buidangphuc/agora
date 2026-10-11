## MODIFIED Requirements

### Requirement: Services refuse unsafe configuration in staging and production

When `ENV` is staging or production:
- team-payment SHALL refuse to start with `MOCK_PAYMENTS=true`.
- team-identity SHALL refuse to start with the committed development signing key, or with
  `PASSWORD_RESET_EXPOSE_TOKEN=true`.
- team-order and team-search SHALL refuse to start without durable storage.
- team-gateway SHALL refuse to start with `EDGE_REFLECTION_ENABLED=true`.

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

#### Scenario: team-identity refuses an exposed reset token in staging

- **WHEN** the team-identity image is started with `ENV=staging`, its own non-development signing key, and
  `PASSWORD_RESET_EXPOSE_TOKEN=true`
- **THEN** the process exits non-zero and its log names `PASSWORD_RESET_EXPOSE_TOKEN`

## ADDED Requirements

### Requirement: Deployed overlays declare ENV for every service with a strict guard

The staging and prod overlays in `platform-gitops/envs/` SHALL set `ENV` (`staging` / `production`) for every service
that has a strict-ENV guard: team-gateway, team-identity, team-order, team-payment, team-search and team-domain. A
missing `ENV` would silently run the service with local defaults.

#### Scenario: The rendered gateway knows its environment

- **WHEN** `charts/service` is rendered for team-gateway against the staging overlay and again against the prod overlay
- **THEN** the gateway container has `ENV` "staging" and "production" respectively

### Requirement: team-ai accepts traffic only from the gateway and Prometheus

The team-ai deployment SHALL ship an ingress NetworkPolicy that admits only pods labelled `app: team-gateway` (its
only gRPC caller) and `app: prometheus` (the KEDA scaling metric source).

#### Scenario: team-ai renders an ingress allow-list

- **WHEN** `charts/service` is rendered for team-ai with its service values
- **THEN** a `NetworkPolicy` selects `app: team-ai` and its only ingress sources are `app: team-gateway` and
  `app: prometheus`
