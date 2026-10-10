# ai-assistant Specification

## Purpose

The AI assistant capability exposes team-ai's three AI features to the marketplace
through the standard edge path: the frontend calls the gateway (Connect), the gateway
forwards to team-ai over gRPC (verifying the JWT once and forwarding `x-principal-*`),
and team-ai serves `platform.ai.v1.AIService`. The model backend is pluggable
(`CHAT_BACKEND=mock` by default; `llm_router` for a real LLM). The frontend never calls
team-ai directly.

## Requirements

### Requirement: Shopping Assistant chatbot answers through the gateway

The system SHALL let a buyer ask the AI shopping assistant a question at `/assistant` and
receive a grounded reply, product cards, and suggested follow-ups produced by `team-ai`
(`platform.ai.v1.AIService/ShoppingAssistant`), routed exclusively through the gateway — never
called directly from the browser.

#### Scenario: Buyer asks the assistant for a recommendation

- **WHEN** a logged-in buyer opens `/assistant` and asks for a product recommendation
- **THEN** the assistant returns a reply referencing catalog products, with product cards and
  follow-up suggestions, sourced from team-ai via the gateway (not a client-side mock)

### Requirement: Shopping Assistant reply streams token-by-token

The system SHALL stream the assistant's reply text to the browser as it is produced, via
`platform.chat.v1.ChatService/StreamChat` (team-ai's token streamer) routed through the gateway
as a Connect server-stream and relayed to the browser by a Next.js route handler. Product cards
and follow-ups are still resolved via the unary `ShoppingAssistant` call. With `CHAT_BACKEND=mock`
the stream is a deterministic echo; a real reply streams when `CHAT_BACKEND=llm_router`.

#### Scenario: Assistant reply appears progressively

- **WHEN** a buyer sends a message on `/assistant`
- **THEN** the reply text appears incrementally (token deltas) rather than only after the full
  answer is ready, streamed from team-ai through the gateway

### Requirement: Magic Listing fills the seller form through the gateway

The system SHALL let a seller on `/seller/new`, after entering a title, trigger "AI Tạo Mô Tả"
to fill the description, suggested category, and price range from `team-ai`
(`platform.ai.v1.AIService/MagicListing`) via the gateway.

#### Scenario: Seller generates a listing description

- **WHEN** a logged-in seller enters a title and clicks the AI generate button on the
  new-listing form
- **THEN** the description and a suggested price range are filled from team-ai via the gateway
  (not a hardcoded template)

### Requirement: Chat Copilot suggests seller replies through the gateway

The system SHALL offer a seller, inside a buyer conversation, up to three quick reply
suggestions from `team-ai` (`platform.ai.v1.AIService/ChatCopilot`), routed through the gateway.

#### Scenario: Seller sees copilot reply suggestions

- **WHEN** a logged-in seller opens a buyer conversation
- **THEN** up to three quick-reply suggestions from team-ai are shown, retrieved via the gateway

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
