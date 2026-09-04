@auth @jwt @jwks
Feature: RS256 JWT signing and JWKS verification
  team-identity signs RS256 JWTs with a private key and a kid, publishes its
  public keys at a JWKS endpoint, and team-gateway verifies incoming tokens
  against the cached JWKS by kid — with no shared HS256 secret and support for
  key rotation without a redeploy.

  Scenario: Login issues an RS256 token carrying a kid
    Given a registered user
    When the user logs in successfully through team-identity
    Then the returned JWT header has alg RS256 and a non-empty kid
    And the token claims still carry sub, name, typ and scopes

  Scenario: The HS256 shared-secret signing path is gone
    Given team-identity is configured with an RSA private key and a kid
    When team-identity starts
    Then it does not read or require a shared JWT_SECRET
    And it cannot mint an HS256 token

  Scenario: JWKS serves the active public key
    Given team-identity has issued a fresh token with a kid
    When a client requests GET /.well-known/jwks.json from team-identity
    Then the JWKS keys array contains an RSA public JWK whose kid matches the token kid
    And no private key material is present in the response

  Scenario: A valid RS256 token from identity is accepted
    Given a bearer token freshly issued by team-identity
    When a request reaches the gateway with that token
    Then the gateway verifies it against the JWKS public key matching the token kid
    And the resolved x-principal metadata is forwarded to the upstream service

  Scenario: Gateway rejects a token not signed by a current JWKS key
    Given a bearer token whose kid is absent from the JWKS
    When a request reaches the gateway with that token
    Then the gateway does not accept it as an authenticated principal
    And the caller is treated as anonymous with PUBLIC_SCOPES

  Scenario: A token signed by a rotated-in key verifies
    Given team-identity has published a new signing key with a new kid alongside the previous key
    And the gateway has refreshed its cached JWKS
    When a request reaches the gateway with a token signed by the new kid
    Then the gateway finds the matching public key in the refreshed JWKS and accepts the token
    And tokens signed by the still-published previous kid also continue to verify
