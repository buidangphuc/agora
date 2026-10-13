// Package outbox implements the transactional outbox: EnqueueTx stores an event in
// the caller's transaction and Relayer publishes the stored rows to Kafka with
// retry, exponential backoff and parking. Delivery is at-least-once, so consumers
// dedupe on the event_id header. Each record carries the headers event_id,
// event_type, x-request-id and traceparent.
//
// Order is guaranteed per aggregate (aggregate_type, aggregate_id): no row is
// published while an older row of the same aggregate is unpublished, including a
// row backing off or parked. Other aggregates are unaffected. Order follows the
// seq column, assigned at insert; two transactions that enqueue for the same
// aggregate concurrently can commit in the other order, so serialise writers of
// one aggregate (a row lock on the aggregate) when that matters.
//
// A parked row blocks its aggregate by design. The relayer logs it at ERROR;
// fix the cause and set the row back to status 'pending' (or delete it) to
// resume. Call Store.Purge periodically to delete old published rows.
package outbox
