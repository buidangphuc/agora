## ADDED Requirements

### Requirement: Real-time listing content indexing for AI RAG

The system SHALL provide a Kafka consumer in `team-ai` subscribing to `listing.events` that continuously indexes created/updated listings into the RAG vector store and removes deleted listings.

#### Scenario: Created listing is indexed into RAG

- **WHEN** a `ListingChanged` event with action `CREATED` or `UPDATED` arrives on `listing.events`
- **THEN** the consumer extracts listing content, indexes it via `KnowledgeRetrievalService`, making it retrievable in AI RAG search

#### Scenario: Deleted listing is removed from RAG

- **WHEN** a `ListingChanged` event with action `DELETED` arrives on `listing.events`
- **THEN** the consumer removes the document from the vector store by `listing_id`
