# proto/ — vendored platform contract

Pinned copy of platform-core's proto module (package `platform.*`), input to
`buf generate` → `../generated/` (gitignored). Per ADR-0001, team-analytics
vendors the proto sources and generates its own Go code; platform-core never
writes here.

team-analytics OWNS the read-only `platform.analytics.v1.AnalyticsQueryService`
(served on `:50059`) and otherwise consumes Kafka events. It vendors
the whole `platform/` tree (18 packages, 19 `.proto` files: `ai`, `analytics`, `audit`,
`chat`, `common`, `engagement`, `events`, `identity`, `listing`, `notification`, `order`,
`payment`, `promotion`, `recommendation`, `referral`, `search`, `sharing`, `verification`),
of which it consumes `common`, `events` and `analytics` to read `analytics.events`:
unmarshal `platform.events.v1.EventEnvelope`, switch on `type`, and decode the
`platform.analytics.v1.TrackingEvent` payload. It does NOT edit the contract —
the analytics contract was added by `add-tracking-event-contract` and lives in
platform-core.

Regenerate: `make proto` then `go mod tidy`.
