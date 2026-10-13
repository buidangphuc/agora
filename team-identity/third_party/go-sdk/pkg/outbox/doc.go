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
// A publish error is retried with a capped backoff and never parks the row,
// unless it is permanent for the record: the broker rejected the record itself
// (message or request too large, corrupt, invalid record or timestamp) or a
// custom Publisher wrapped the error in Permanent. Errors about the topic or the
// cluster (unknown topic, authorization) retry too, with an ERROR log once
// RelayerOptions.AlertAfter attempts have failed, because fixing them needs no
// change to the rows. A parked row is logged at ERROR and blocks its aggregate
// until Store.Requeue, Store.RequeueParked (bulk, by topic or error text) or
// deleting the row. Watch Store.Stats: Parked above zero needs a person. Call
// Store.Purge periodically to delete old published rows.
//
// A table made by an SDK older than v1 must be migrated first; see SchemaSQL.
package outbox
