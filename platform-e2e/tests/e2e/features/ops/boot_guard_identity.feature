@auth
Feature: team-identity refuses the development signing key outside local
  As an operator
  I want team-identity to refuse to boot in production with the committed development key
  So that tokens signed by a publicly known key can never be issued in production

  # Black box: the real team-identity image is run with `docker run --rm` on the stack
  # network (it opens Postgres before the signing-key guard), with the compose env.
  Scenario: team-identity refuses the development signing key in production
    When the team-identity image is started with ENV=production and the development signing key from the local compose file
    Then the process exits non-zero and its log names the signing key

  @destructive
  Scenario: team-identity refuses an exposed reset token in staging
    When the team-identity image is started with ENV=staging, its own non-development signing key, and PASSWORD_RESET_EXPOSE_TOKEN=true
    Then the identity process exits non-zero and its log names PASSWORD_RESET_EXPOSE_TOKEN
