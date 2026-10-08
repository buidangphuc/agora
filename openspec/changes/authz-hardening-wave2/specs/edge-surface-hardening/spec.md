## Purpose

Defines what the public gateway reveals and accepts at its edge: no raw upstream internals in error messages, no unauthenticated
schema discovery in deployed environments, and no token without an expiry.

## ADDED Requirements

### Requirement: Upstream server-side errors are sanitised at the edge

For every proxied unary and streaming call the gateway SHALL replace the message of an upstream error whose code is
`Internal`, `Unknown`, `DataLoss` or `Unavailable`, and of any non-status transport error, with a fixed generic message
(`internal error` for Internal, Unknown, DataLoss and non-status errors; `service unavailable` for Unavailable), SHALL keep the
code, SHALL return the request id in the `X-Request-Id` response metadata, and SHALL log the original message together with
that request id. Errors with a client-meaningful code (`InvalidArgument`, `NotFound`, `AlreadyExists`, `PermissionDenied`,
`Unauthenticated`, `FailedPrecondition`, `ResourceExhausted`, `Aborted`, `OutOfRange`, `Unimplemented`, `Canceled`,
`DeadlineExceeded`) SHALL keep their upstream message.

#### Scenario: A raw internal error does not reach the client

- **WHEN** an upstream service answers `Internal` with a message containing a driver or SQL error text
- **THEN** the client receives code `internal` with message `internal error` and an `X-Request-Id`, and the gateway log line for that request id contains the original message

#### Scenario: An unavailable upstream does not leak its cause

- **WHEN** an upstream answers `Unavailable` with a message naming an internal host
- **THEN** the client receives code `unavailable` with message `service unavailable`

#### Scenario: A client-meaningful message is preserved

- **WHEN** an upstream answers `InvalidArgument` with message `amount must be positive`, then `FailedPrecondition` with `insufficient wallet balance`
- **THEN** the client receives those exact messages with the same codes

#### Scenario: Streaming calls are sanitised too

- **WHEN** an upstream streaming call fails with `Internal` and a raw message
- **THEN** the client receives `internal error`

### Requirement: gRPC reflection is off unless enabled for development

The gateway SHALL serve gRPC server reflection only when `EDGE_REFLECTION_ENABLED` is true (default false) and SHALL refuse to
start when it is true and `ENV` is staging, stage, prod or production. With it disabled, reflection paths SHALL answer
`unimplemented` or not found like any unknown route.

#### Scenario: Reflection is unavailable by default

- **WHEN** a client calls the gRPC reflection endpoint on a gateway started without `EDGE_REFLECTION_ENABLED`
- **THEN** the call fails as unimplemented or not found and no service names are listed

#### Scenario: Reflection works when enabled locally

- **WHEN** the gateway runs with `EDGE_REFLECTION_ENABLED=true` and `ENV=local`
- **THEN** the reflection endpoint lists the registered services

#### Scenario: Reflection cannot be enabled in a deployed environment

- **WHEN** the gateway starts with `EDGE_REFLECTION_ENABLED=true` and `ENV=production`
- **THEN** startup fails with an error naming `EDGE_REFLECTION_ENABLED`

### Requirement: Tokens without an expiry or subject are anonymous

The gateway verifier SHALL treat a token that lacks an `exp` claim or has an empty `sub` as invalid, resolving the caller to the
anonymous principal exactly like any other invalid token. Tokens issued by identity (always carrying `exp` and `sub`) SHALL
verify as before.

#### Scenario: A token without exp is anonymous

- **WHEN** a correctly signed token with a valid `kid` but no `exp` is presented
- **THEN** the caller resolves to the anonymous principal

#### Scenario: A token without a subject is anonymous

- **WHEN** a correctly signed, unexpired token with an empty `sub` is presented
- **THEN** the caller resolves to the anonymous principal

#### Scenario: A normal identity token still verifies

- **WHEN** a buyer logs in and presents the issued token
- **THEN** the caller resolves to that buyer with their scopes
