## ADDED Requirements

### Requirement: OpenFeature + Flipt provider is wired into the Go services

The system SHALL initialize an OpenFeature client backed by the Flipt provider inside a Go
service's bootstrap/lifecycle (opened on startup, closed on shutdown), evaluating flags
against an **in-memory snapshot** that Flipt keeps fresh over a gRPC stream, so a flag check
is an in-process lookup rather than a per-request network call. `team-order` is the
representative service.

#### Scenario: The service starts with a working flag client

- **WHEN** `team-order` boots with `FLIPT_ADDR` pointing at the Flipt server
- **THEN** its bootstrap constructs an OpenFeature client with the Flipt provider, the client
  is available to handlers, and the provider maintains an in-memory flag snapshot streamed
  from Flipt (no per-evaluation round-trip)

#### Scenario: The flag client shuts down cleanly

- **WHEN** `team-order` shuts down
- **THEN** the OpenFeature provider / Flipt stream is closed as part of resource teardown

### Requirement: OpenFeature + Flipt provider is wired into the frontend, server-side only

The system SHALL evaluate feature flags in `team-frontend` using an OpenFeature client with
the Flipt provider **only** in server-side code (Server Components / Server Actions), never in
the browser, so the client never learns the Flipt endpoint and never evaluates flags itself.

#### Scenario: The browser never evaluates flags

- **WHEN** a page whose UI depends on a flag is rendered
- **THEN** the flag is evaluated on the server and the browser receives already-resolved
  markup, with no flag SDK, flag state, or Flipt address shipped to the client

### Requirement: Checkout has an emergency kill-switch that flips without redeploy

The system SHALL gate the checkout / place-order path on a boolean flag `checkout-enabled`
evaluated against Flipt, such that flipping the flag off in Flipt disables checkout — hidden
in the UI and rejected by `team-order` — within seconds, with no PR, image rebuild, or pod
restart, and flipping it back on restores checkout the same way. The flag SHALL default to
**on** so that a Flipt outage does not disable checkout.

#### Scenario: Kill-switch off blocks checkout end-to-end

- **WHEN** `checkout-enabled` is turned off in Flipt and a buyer tries to check out
- **THEN** the checkout / place-order entry point is hidden or disabled in the UI, and if a
  `CreateOrder` request reaches `team-order` it is rejected with a clear "checkout
  unavailable" gRPC error instead of running the purchase saga

#### Scenario: Kill-switch on allows checkout

- **WHEN** `checkout-enabled` is on (or Flipt is unreachable and the default applies)
- **THEN** the checkout entry point is shown and a buyer can place an order through the normal
  gateway → team-order path

#### Scenario: Toggling takes effect without a redeploy

- **WHEN** an operator flips `checkout-enabled` in the Flipt UI while the services and frontend
  are running unchanged
- **THEN** the new state takes effect within seconds through the streamed in-memory snapshot,
  with no code change, image rebuild, ArgoCD sync, or pod restart
