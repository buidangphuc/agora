// Package outbox implements the transactional outbox: EnqueueTx stores an event in
// the caller's transaction and Relayer publishes the stored rows to Kafka with
// retry, capped exponential backoff and parking. Delivery is at-least-once, so
// consumers dedupe on the event-id header. Each record carries the kafkax headers
// event-id, event-type, x-request-id and traceparent.
//
// Order is guaranteed per aggregate (aggregate_type, aggregate_id): no row is
// published while an older row of the same aggregate is unpublished, including a
// row backing off or parked. Other aggregates are unaffected. Order follows the
// seq column, assigned at insert; two transactions that enqueue for the same
// aggregate concurrently can commit in the other order, so serialise writers of
// one aggregate (a row lock on the aggregate) when that matters. Across
// aggregates there is no order.
//
// A transient publish error (broker down, timeout) is retried with a capped
// backoff and never parks the row. A non-retriable error (record too large,
// unknown topic, authorization) parks it: the relayer logs it at ERROR and its
// aggregate stays blocked until Store.Requeue (or deleting the row). Watch
// Store.Stats: Parked above zero needs a person. Call Store.Purge periodically
// to delete old published rows.
//
// A table made by an SDK older than v1 must be migrated first; see SchemaSQL.
package outbox
