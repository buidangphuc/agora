## ADDED Requirements

### Requirement: An SSE connection ends when its token expires or its session is revoked

For every `/api/events/live` connection to a room that requires a credential (`user:*`, `chat:*`, `ops:*`),
`team-gateway` SHALL end the response when the credential's `exp` passes while the connection is open, and within
`STREAM_REVOCATION_CHECK_SECONDS` (default 5) after the token's session appears in the gateway's in-memory revocation
denylist. Ending SHALL write one final SSE event named `unauthenticated` (data carries `code` `unauthenticated` and a
`reason` of `token_expired` or `session_revoked`), then close the response, which releases the broker subscription so
nothing more is streamed. A connection the client closes first SHALL get no final event. Public rooms (`global`,
`listing:*`) and requests without a credential SHALL be unchanged. A reconnect with the expired or revoked token SHALL
be refused with HTTP 401 at open.

#### Scenario: An SSE connection is ended when its token expires

- **WHEN** a logged-in buyer whose token expires in about 3 seconds opens `/api/events/live` for their own `user:{id}` room
- **THEN** the gateway sends an `unauthenticated` event about when the token expires and closes the connection

#### Scenario: An SSE connection is ended when its session is revoked

- **WHEN** a logged-in buyer opens `/api/events/live` for their own `user:{id}` room on a gateway that checks revocation
  every second, then revokes that session through `RevokeSession`
- **THEN** the gateway sends an `unauthenticated` event and closes the connection within 4 seconds of the revoke

#### Scenario: A public SSE room is unaffected by token expiry

- **WHEN** a client opens `/api/events/live` for a public `listing:{id}` room with no credential and stays connected
  past the time a 3 second token would have expired
- **THEN** the connection is still open and receives no `unauthenticated` event
