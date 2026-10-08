## Purpose

Defines how `team-ai` binds recommendation requests to the caller and how its gRPC surface authenticates, so a caller cannot
request another user's personalised recommendations and no static shared token grants admin scopes in a deployed environment.

## ADDED Requirements

### Requirement: Recommendations are bound to the caller's identity

`Recommend` SHALL, for a principal of type `user`, use the principal id as the user id and ignore the request `user_id` and
`anonymous_id`. For the anonymous principal it SHALL ignore the request `user_id` (treated as empty) and MAY use the request
`anonymous_id`. For a principal holding `admin` or of type `service` the request `user_id` SHALL be honoured. The `recommendations:read`
scope requirement, the response shape and the cold-start behaviour SHALL be unchanged.

#### Scenario: A user cannot request another user's recommendations

- **WHEN** buyer A calls `Recommend` with `user_id` set to buyer B
- **THEN** the recommendation is computed for buyer A (B's precomputed list is not used)

#### Scenario: A user's own request is unchanged

- **WHEN** buyer A calls `Recommend` with `user_id` equal to A's id, or with an empty `user_id`
- **THEN** A's personalised list is used in both cases

#### Scenario: Anonymous callers cannot impersonate a user

- **WHEN** a caller holding the anonymous principal with `recommendations:read` calls `Recommend` with `user_id` set to a real user
- **THEN** the request is served as an anonymous request (cold-start path)

#### Scenario: Admin may request on behalf of a user

- **WHEN** the seeded admin calls `Recommend` with `user_id` set to a user
- **THEN** that user's list is used

### Requirement: The static bearer fallback is disabled unless explicitly enabled for local use

The gRPC authentication interceptor SHALL accept `authorization: bearer <AUTH_BEARER_TOKEN>` only when
`GRPC_BEARER_FALLBACK_ENABLED` is true (default false). The service SHALL refuse to start when that flag is true and
`ENVIRONMENT` is not a local environment (dev, local, test). With the fallback disabled, a call without gateway-forwarded
`x-principal-*` metadata SHALL be `UNAUTHENTICATED` even if a bearer token is presented. `AUTH_ROLES` SHALL default to empty, so
a fallback principal holds no scopes unless roles are configured explicitly. The HTTP bearer authentication of the REST API
SHALL be unchanged.

#### Scenario: A bearer token is ignored on gRPC by default

- **WHEN** a direct caller presents the configured bearer token with no `x-principal-*` metadata and the flag is unset
- **THEN** the call is `UNAUTHENTICATED`

#### Scenario: The fallback works when enabled locally

- **WHEN** `GRPC_BEARER_FALLBACK_ENABLED=true` in a local environment and a caller presents the configured bearer token
- **THEN** the call resolves the fallback principal with the configured `AUTH_ROLES` scopes

#### Scenario: The fallback cannot be enabled outside local

- **WHEN** the service starts with `GRPC_BEARER_FALLBACK_ENABLED=true` and `ENVIRONMENT=production`
- **THEN** startup fails with an error naming `GRPC_BEARER_FALLBACK_ENABLED`

#### Scenario: Gateway-forwarded principals are unaffected

- **WHEN** a call arrives with gateway-forwarded `x-principal-*` metadata
- **THEN** that principal is used regardless of the fallback flag
