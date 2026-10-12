Code track only: every scenario gets a `not-testable` FEATURES entry.

## 1. SDK v0.2.1 (single packet)

Owns: `platform-core/packages/go-sdk/pkg/{outbox,upstream,flags,consume,kafkax}/**`, and the box ticks.

- [ ] 1.1 Red tests first for the five scenarios. `NewConsumerClient` gets a signature change to `(c config.Kafka, group string, topics []string, opts ...kgo.Opt)`; update all its in-SDK callers, then implement them. `make check` must be green with Docker.

## 2. Close (orchestrator)

- [ ] 2.1 Add FEATURES entries, run `ci_local`, and tag `go-sdk/v0.2.1` locally.
