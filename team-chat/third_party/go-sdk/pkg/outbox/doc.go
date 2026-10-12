// Package outbox implements the transactional outbox: EnqueueTx stores an event in
// the caller's transaction and Relayer publishes the stored rows to Kafka with
// retry, exponential backoff and parking. Delivery is at-least-once, so consumers
// dedupe on event_id.
package outbox
