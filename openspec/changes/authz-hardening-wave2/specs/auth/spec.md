## ADDED Requirements

### Requirement: The admin account is seeded only from an explicit password and never with a known default in a deployed environment

`team-identity` SHALL seed the admin account only when `ADMIN_PASSWORD` is set, using that value; when it is unset no admin SHALL
be seeded. When `ENV` is staging, stage, prod or production the service SHALL refuse to start if `ADMIN_PASSWORD` equals
`admin123`. Local compose SHALL set `ADMIN_PASSWORD` explicitly so the seeded admin used by tests keeps working. The admin
scope SHALL still be obtainable only by the seeded admin.

#### Scenario: No admin without a password

- **WHEN** identity starts with `ADMIN_PASSWORD` unset against an empty database
- **THEN** no admin user exists and `Login` for `admin` fails

#### Scenario: A known default password is refused in a deployed environment

- **WHEN** identity starts with `ENV=production` and `ADMIN_PASSWORD=admin123`
- **THEN** startup fails with an error naming `ADMIN_PASSWORD`

#### Scenario: Local compose keeps the seeded admin

- **WHEN** the local stack starts with `ADMIN_PASSWORD=admin123` and `ENV=local`
- **THEN** `Login` as `admin` with `admin123` succeeds and the token carries `admin`

#### Scenario: A deployed environment seeds its own admin

- **WHEN** identity starts with `ENV=staging` and a non-default `ADMIN_PASSWORD`
- **THEN** the admin is seeded with that password and `admin123` no longer logs in

### Requirement: The committed development signing key is refused in deployed environments

When `ENV` is staging, stage, prod or production, `team-identity` SHALL refuse to start if `JWT_KID` is `dev-2026` or the private
key's fingerprint equals the development key committed to the repository. The committed development key SHALL be documented as
development-only and as requiring rotation in any shared environment.

#### Scenario: The dev key is refused in production

- **WHEN** identity starts with `ENV=production` and the committed development private key
- **THEN** startup fails with an error naming the signing key

#### Scenario: The dev kid is refused even with another key

- **WHEN** identity starts with `ENV=staging`, a freshly generated key and `JWT_KID=dev-2026`
- **THEN** startup fails with an error naming `JWT_KID`

#### Scenario: The dev key still works locally

- **WHEN** identity starts with `ENV=local` and the committed development key
- **THEN** it starts and publishes the key at the JWKS endpoint

### Requirement: Password reset tokens are never returned to the requester by default

`RequestPasswordReset` SHALL NOT include the reset token in its response unless `DEV_RETURN_RESET_TOKEN` is true (default false);
the service SHALL refuse to start when that flag is true and `ENV` is staging, stage, prod or production. When the token is
withheld the service SHALL record the issuance in its log with the user id, expiry and a non-reversible token fingerprint and
never the raw token. `ResetPassword` SHALL keep accepting a valid token. `ChangePassword` SHALL take the user id from the
principal when present, ignoring a different request `user_id` for a non-admin principal.

#### Scenario: The response carries no token by default

- **WHEN** `RequestPasswordReset` is called for an existing user with the flag unset
- **THEN** the response has an empty `reset_token` and the log records the issuance without the raw token

#### Scenario: The token is returned only under the dev flag

- **WHEN** `DEV_RETURN_RESET_TOKEN=true` with `ENV=local` and `RequestPasswordReset` is called
- **THEN** the response carries the token and `ResetPassword` with it succeeds once

#### Scenario: The dev flag is refused in production

- **WHEN** identity starts with `DEV_RETURN_RESET_TOKEN=true` and `ENV=production`
- **THEN** startup fails with an error naming `DEV_RETURN_RESET_TOKEN`

#### Scenario: ChangePassword uses the caller's identity

- **WHEN** buyer A calls `ChangePassword` with `user_id` set to buyer B and A's old password
- **THEN** the call acts on A (or fails on B's old password) and B's password is unchanged

### Requirement: Secrets are provisioned only through the ADR-0006 path

The repository SHALL NOT contain a script that seeds a shared symmetric JWT secret or grants every service read access to the
signing material. The GitOps vault configuration, where only `team-identity` may read the signing key, SHALL be the single
documented provisioning path.

#### Scenario: No legacy shared-secret seeding script remains

- **WHEN** the repository is searched for `JWT_SECRET` seeding and for a policy granting every service `svc/data/shared/jwt`
- **THEN** no such script or policy exists, and deployment documentation points to the GitOps vault configuration
