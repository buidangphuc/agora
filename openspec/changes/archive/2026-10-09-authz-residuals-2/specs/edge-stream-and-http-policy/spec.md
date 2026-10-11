## ADDED Requirements

### Requirement: A stream ends when its token expires or its session is revoked

For every Connect server stream the gateway routes (today `ChatService/StreamChat`), `team-gateway` SHALL end the stream
with the Connect error code `unauthenticated` when the bearer token's `exp` passes while the stream is open. It SHALL also
end it with `unauthenticated`, within `STREAM_REVOCATION_CHECK_SECONDS` (default 5) after the session appears in the
gateway's in-memory revocation denylist, when the token's session is revoked while the stream is open. Ending the stream
SHALL also cancel the upstream call. A stream that finishes before either event SHALL be unaffected. The check uses only
the token and the denylist the gateway already holds; if the denylist source is unavailable only the expiry is
enforced.

#### Scenario: A stream is ended when its token expires

- **WHEN** a logged-in buyer whose token expires in about 3 seconds opens a `StreamChat` the model provider holds open
- **THEN** the stream ends with the Connect error code `unauthenticated` about when the token expires, not after the
  provider answers

#### Scenario: A stream is ended when its session is revoked

- **WHEN** a logged-in buyer opens a `StreamChat` the model provider holds open on a gateway that checks revocation
  every second, then revokes that session through `RevokeSession`
- **THEN** the stream ends with the Connect error code `unauthenticated` within 4 seconds of the revoke, before the
  provider's own timeouts end it

#### Scenario: A stream that finishes before expiry is unaffected

- **WHEN** a logged-in buyer whose token expires in about 60 seconds calls `StreamChat` and the provider answers at once
- **THEN** the stream completes with chat text and no error
