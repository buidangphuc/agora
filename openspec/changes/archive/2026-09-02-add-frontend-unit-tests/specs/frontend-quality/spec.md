## ADDED Requirements

### Requirement: team-frontend has a unit test runner

`team-frontend` SHALL provide a unit test runner (Vitest + React Testing Library, jsdom environment)
invoked by a `test` script, so isolated tests run in seconds without the full stack. The runner SHALL
be wired so it can run in the existing quality pipeline alongside Biome and `tsc`.

#### Scenario: Test script runs the unit suite

- **WHEN** a developer runs `npm test` (`vitest run`) in `team-frontend`
- **THEN** the Vitest suite executes against jsdom and reports pass/fail, with no dependency on a running
  gateway or backend

### Requirement: Server Actions and gateway wrappers are unit-tested

The Server Actions in `src/features/*/actions.ts` (all nine: address, assistant, auth, cart, chat,
engagement, listing, order, review) and the gateway client wrappers in `src/lib/gateway/*.ts` SHALL have
unit tests that mock the ConnectRPC transport and Next.js request APIs, asserting input validation,
that the correct gateway call is made, and that success and error results are shaped as callers expect.

#### Scenario: An action's success and error paths are covered

- **WHEN** the suite runs a test for a Server Action with the gateway wrapper mocked to succeed, and again
  mocked to fail
- **THEN** the success case asserts the action returns the expected result and the correct wrapper was
  called with mapped inputs, and the failure case asserts the action returns the expected error shape
  (no unhandled rejection)

#### Scenario: A gateway wrapper maps to its client call

- **WHEN** the suite runs a test for a `src/lib/gateway/*.ts` wrapper with its ConnectRPC client mocked
- **THEN** it asserts the wrapper invokes the right RPC with the mapped request and normalizes the
  response/error, without a real network call
