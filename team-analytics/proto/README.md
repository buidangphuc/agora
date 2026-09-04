# proto/ — vendored platform contract

Pinned copy of platform-core's proto module (package `platform.*`), input to
`buf generate` → `../generated/` (gitignored). Per ADR-0001, team-analytics
vendors the proto sources and generates its own Go code; platform-core never
writes here.

team-analytics OWNS no RPC surface (it is a Kafka-consumer worker). It vendors
`common`, `events`, and `analytics` so it can consume `analytics.events`:
unmarshal `platform.events.v1.EventEnvelope`, switch on `type`, and decode the
`platform.analytics.v1.TrackingEvent` payload. It does NOT edit the contract —
the analytics contract was added by `add-tracking-event-contract` and lives in
platform-core.

Regenerate: `make proto` then `go mod tidy`.
