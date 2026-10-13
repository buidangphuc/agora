// Package consume runs a Kafka consumer-group loop that never commits past a
// record it has neither handled nor dead-lettered, and provides the per-consumer
// transactional dedupe ledger that makes redelivery safe.
package consume
