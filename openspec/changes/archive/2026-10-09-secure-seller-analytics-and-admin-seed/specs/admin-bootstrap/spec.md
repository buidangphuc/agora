## Purpose

Defines how the first admin account is created in `team-identity`.

## ADDED Requirements

### Requirement: The admin account is seeded only when explicitly enabled with a supplied password

`team-identity` SHALL NOT create any account with a built-in password. It SHALL seed an
admin only when `SEED_ADMIN_ENABLED=true`, using `SEED_ADMIN_USERNAME` (default `admin`)
and `SEED_ADMIN_PASSWORD`, which has no default. If seeding is enabled and the password is
empty or shorter than 12 characters, the service SHALL fail to start with a clear error.
Seeding is off by default; the local compose stack enables it with a dev-only password
from its env, and deployment manifests SHALL NOT enable it (an operator creates the first
admin out of band, or enables it once with a secret).

#### Scenario: Default configuration creates no admin

- **WHEN** team-identity starts with no `SEED_ADMIN_*` variables against an empty database
- **THEN** no admin user exists and a login as `admin` / `admin123` fails

#### Scenario: Enabled seeding without a password refuses to start

- **WHEN** team-identity starts with `SEED_ADMIN_ENABLED=true` and no `SEED_ADMIN_PASSWORD`
- **THEN** the process exits non-zero with an error naming `SEED_ADMIN_PASSWORD`

#### Scenario: Local stack seeds the dev admin from env

- **WHEN** the local compose stack starts with `SEED_ADMIN_ENABLED=true` and a dev password
  set in its env
- **THEN** the e2e admin login with those credentials succeeds

#### Scenario: Deployment manifests never enable seeding

- **WHEN** the platform-gitops manifests for team-identity are rendered
- **THEN** they contain no `SEED_ADMIN_ENABLED=true` and no admin password literal
