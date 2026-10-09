@auth @admin
Feature: The first admin account is seeded only when explicitly enabled with a supplied password
  As an operator
  I want team-identity to never ship a built-in credential
  So that an admin exists only where someone chose a password for it

  # Black box: the real team-identity image runs with `docker run` on the stack network, with
  # the local compose environment minus/plus the setting under test. The "no admin" case runs
  # against a scratch database created for the scenario (migrated with the real migrations) so
  # the stack's own identity database is never touched.
  Scenario: Default configuration creates no admin
    Given a scratch identity database migrated with the real migrations
    When the team-identity image is started against it with no SEED_ADMIN variables
    Then the scratch database holds no admin user and no user named admin
    And a login as admin with the password admin123 fails at the gateway

  Scenario: Enabled seeding without a password refuses to start
    When the team-identity image is started with SEED_ADMIN_ENABLED=true and no SEED_ADMIN_PASSWORD
    Then the identity process exits non-zero and its log names SEED_ADMIN_PASSWORD

  Scenario: Local stack seeds the dev admin from env
    When the e2e admin logs in with the credentials from the local compose environment
    Then the login succeeds and the token carries the admin role

  Scenario: Deployment manifests never enable seeding
    When the platform-gitops manifests for team-identity are rendered for every environment
    Then they contain no SEED_ADMIN_ENABLED=true and no admin password literal
    And the platform-gitops seed check passes on them and fails on a crafted bad sample
