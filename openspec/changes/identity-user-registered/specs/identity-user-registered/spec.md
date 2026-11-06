## ADDED Requirements

### Requirement: Register accepts an optional bounded referral code

`Register` SHALL accept an optional `referral_code`. The system SHALL trim it and keep it only when, after the trim, it matches
`^[A-Za-z0-9_-]{1,32}$` (case kept); otherwise the system SHALL treat it as empty. The referral code SHALL NOT cause
Register to fail and SHALL NOT change the Register response.

#### Scenario: Register with a well-formed code succeeds

- **WHEN** a visitor registers a new username with `referral_code` `AB12cd_-`
- **THEN** Register returns the same success response shape as without a code and the stored `UserRegistered` payload carries `AB12cd_-`

#### Scenario: Register without a code succeeds

- **WHEN** a visitor registers a new username with no `referral_code`
- **THEN** Register succeeds and the `UserRegistered` payload carries an empty `referral_code`

#### Scenario: A malformed code is dropped, not rejected

- **WHEN** a visitor registers with `referral_code` `bad code;DROP TABLE` (space and punctuation outside the charset)
- **THEN** Register succeeds with the same response shape and the `UserRegistered` payload carries an empty `referral_code`

#### Scenario: Surrounding whitespace is trimmed before validation

- **WHEN** a visitor registers with `referral_code` ` AB12 ` (leading and trailing spaces)
- **THEN** Register succeeds and the `UserRegistered` payload carries `AB12`

#### Scenario: A code that is empty after trimming becomes empty

- **WHEN** a visitor registers with `referral_code` of three spaces
- **THEN** Register succeeds and the `UserRegistered` payload carries an empty `referral_code`

#### Scenario: A non-ASCII character is dropped

- **WHEN** a visitor registers with `referral_code` `ABé12`
- **THEN** Register succeeds and the `UserRegistered` payload carries an empty `referral_code`

#### Scenario: Case is preserved

- **WHEN** a visitor registers with `referral_code` `REFa1B2c3D4`
- **THEN** the `UserRegistered` payload carries `REFa1B2c3D4` with its original case

#### Scenario: A client-supplied user id or role escalation is ignored

- **WHEN** a visitor calls Register with `role` `admin`
- **THEN** the created account is a buyer, its user id is server-generated, and the `UserRegistered` `user_id` equals that generated id

#### Scenario: Boundary length of 32 versus 33 characters

- **WHEN** one visitor registers with a 32-character valid code and another registers with a 33-character valid-charset code
- **THEN** the first payload carries the 32-character code and the second payload carries an empty `referral_code`, and both Register calls succeed

#### Scenario: An unknown code never reveals itself

- **WHEN** a visitor registers with a well-formed code that belongs to no user
- **THEN** Register returns a success response with a status code and body shape identical to registering with a known code

### Requirement: Register announces the new user atomically

The system SHALL write one `UserRegistered{user_id, referral_code, registered_at}` row to `identity_outbox_events` in the
same database transaction as the user insert, keyed by the new user id, with envelope type
`platform.identity.v1.UserRegistered` and event id equal to the outbox row id. The relay SHALL publish it to
`identity.events` at least once with a stable event id.

#### Scenario: One event per successful registration

- **WHEN** a visitor registers successfully
- **THEN** exactly one `identity_outbox_events` row with type `platform.identity.v1.UserRegistered` exists for the new user id, and its `registered_at` is within 5 seconds of the user's `created_at`

#### Scenario: A duplicate username writes no event

- **WHEN** a visitor registers a username that already exists
- **THEN** Register returns the existing conflict error and no `UserRegistered` row is added for that attempt

#### Scenario: The user row and the event commit together

- **WHEN** the outbox insert fails during Register (error injected at store level)
- **THEN** Register returns an internal error that is not the conflict error, and no user row is written, so no account exists without its event
- **VERIFIED BY**: `team-identity/internal/store` `TestCreateRegisteredOutboxInsertFailureRollsBackUser`; `team-identity/internal/store` `TestCreateRegisteredEnqueueFailureRollsBackUser`

#### Scenario: Redelivery carries a stable event id

- **WHEN** the relay publishes the same outbox row twice
- **THEN** both Kafka messages carry the same envelope `event_id`, equal to the outbox row id, and the same key (the user id)
- **VERIFIED BY**: `team-identity/internal/events` `TestBuildUserRegisteredEnvelopeStableEventID`

#### Scenario: Register does not depend on downstream services

- **WHEN** a visitor registers while team-referral, team-promotion and Kafka are stopped
- **THEN** Register still returns success within the existing Register latency budget and the outbox row stays pending until the relay can publish

### Requirement: Logs never contain raw ids or codes

Register, the outbox relay and the event builder SHALL NOT write the raw user id or the referral code to logs; they
SHALL use `principal_fp` or `key_fp` fingerprints.

#### Scenario: Register logs contain no raw identifiers

- **WHEN** a visitor registers with a referral code and the captured service and relay logs are searched
- **THEN** neither the new user id nor the referral code appears in any log line

#### Scenario: A failed Register logs no raw identifiers either

- **WHEN** a visitor registers a duplicate username with a referral code and the error path logs
- **THEN** the log lines contain no raw user id, referral code, only fingerprints

### Requirement: The gateway passes the referral code through

The gateway SHALL forward the `referral_code` field of the public `Register` request unchanged to team-identity and
SHALL keep Register public.

#### Scenario: Gateway forwards the field

- **WHEN** a client calls `Register` at the gateway with `referral_code` `AB12cd`
- **THEN** the `UserRegistered` event for the new user carries `AB12cd`

#### Scenario: Register stays public at the edge

- **WHEN** an anonymous client calls `Register` at the gateway with a referral code
- **THEN** the gateway answers with the Register result and not 401 or 403

### Requirement: GetMyAccount returns the caller's own account

`AuthService.GetMyAccount` SHALL take no request fields and SHALL return `{user_id, created_at}` for the calling
Principal only. An anonymous caller SHALL be refused with `UNAUTHENTICATED`. The gateway SHALL route it for any
signed-in user and SHALL refuse an anonymous call with 401.

#### Scenario: A signed-in user reads their own account

- **WHEN** a buyer who registered 2 hours ago calls `GetMyAccount` through the gateway
- **THEN** the response `user_id` equals that buyer's id and `created_at` is within 5 seconds of the registration time

#### Scenario: An anonymous call is refused

- **WHEN** a caller with no token calls `GetMyAccount` at the gateway, and a caller without a Principal calls it directly on team-identity
- **THEN** the gateway answers 401 and team-identity answers `UNAUTHENTICATED`

#### Scenario: A caller can never read another user's account

- **WHEN** buyer B calls `GetMyAccount` with a header or metadata value naming buyer A's id
- **THEN** the response `user_id` is B's own id and nothing of A's account is returned

#### Scenario: A seller and a buyer are both allowed

- **WHEN** a seller and a buyer each call `GetMyAccount` through the gateway
- **THEN** each response carries that caller's own `user_id` and no role or scope beyond a valid token is required

#### Scenario: A principal whose account no longer exists

- **WHEN** a valid token's subject has no user row and the caller invokes `GetMyAccount`
- **THEN** team-identity answers `NOT_FOUND` and returns no account data
