## ADDED Requirements

### Requirement: Real-time listing content indexing for AI RAG

The system SHALL provide a Kafka consumer in `team-ai` subscribing to `listing.events` that continuously indexes created/updated listings into the RAG vector store and removes deleted listings. The consumer SHALL run for the lifetime of the application when enabled, and SHALL tolerate redelivery and failing records without stalling.

#### Scenario: Created listing is indexed into RAG

- **WHEN** a `ListingChanged` event with action `CREATED` or `UPDATED` arrives on `listing.events`
- **THEN** the consumer extracts listing content, indexes it via `KnowledgeRetrievalService`, making it retrievable in AI RAG search
- **VERIFIED BY**: team-ai/tests/unit/modules/test_listing_indexer_consumer.py › test_created_listing_becomes_retrievable_then_deleted_is_gone. Not verifiable end to end: the RAG store has no read path through the gateway (see design.md).

#### Scenario: Deleted listing is removed from RAG

- **WHEN** a `ListingChanged` event with action `DELETED` arrives on `listing.events`
- **THEN** the consumer removes the document from the vector store by `listing_id`
- **VERIFIED BY**: team-ai/tests/unit/modules/test_listing_indexer_consumer.py › test_created_listing_becomes_retrievable_then_deleted_is_gone. Not verifiable end to end: the RAG store has no read path through the gateway (see design.md).

#### Scenario: Unpublished listing is removed from RAG

- **WHEN** a `ListingChanged` event for a listing whose status is `DRAFT` or `REJECTED` arrives
- **THEN** the listing is removed from the vector store instead of indexed
- **VERIFIED BY**: team-ai/tests/unit/modules/test_listing_indexer_consumer.py › test_unpublished_status_is_removed_from_rag. Not verifiable end to end: the RAG store has no read path through the gateway (see design.md).

#### Scenario: Redelivered event is applied once

- **WHEN** the same event (same `event_id`) is delivered twice, or an event older than the last one applied to that listing arrives
- **THEN** the second delivery causes no second index or delete
- **VERIFIED BY**: team-ai/tests/unit/modules/test_listing_indexer_consumer.py › test_redelivered_event_is_applied_once, test_event_older_than_applied_one_is_skipped. Not verifiable end to end: redelivery cannot be forced through the gateway and the RAG store has no read path there.

#### Scenario: A failing event is retried and then parked

- **WHEN** indexing an event keeps failing, or an event cannot be decoded
- **THEN** the consumer retries a failing event up to `LISTING_INDEXER_MAX_ATTEMPTS` times, then produces the record to `listing.events.dlq` and commits its offset, and an undecodable record goes to the DLQ without retries
- **AND** a record that can be neither applied nor parked is not committed
- **VERIFIED BY**: team-ai/tests/unit/modules/test_listing_indexer_consumer.py › test_exhausted_retries_park_on_dlq_and_stream_continues, test_undecodable_record_goes_straight_to_dlq, test_unparkable_record_is_not_committed. Not verifiable end to end: failing the embedding backend or the DLQ needs fault injection the stack does not expose.

#### Scenario: The consumer runs with the application

- **WHEN** `LISTING_INDEXER_ENABLED=true` and `RAG_ENABLED=true`
- **THEN** application startup starts the consumer on `listing.events` (group `team-ai-indexer`), a crashed consumer is restarted, and shutdown stops it
- **AND** `LISTING_INDEXER_ENABLED=true` without `RAG_ENABLED` fails startup
- **VERIFIED BY**: team-ai/tests/unit/modules/test_listing_indexer_consumer.py › test_addon_is_gated_and_starts_and_stops_the_consumer, test_supervisor_restarts_a_failed_consumer, test_indexer_requires_rag. Not verifiable end to end: the effect of the consumer is only visible in the RAG store (see design.md).
