## ADDED Requirements

### Requirement: Shopping Assistant is grounded in indexed listings

When a RAG store is configured, `AIService.ShoppingAssistant` SHALL answer from the listings the listing indexer put in it: each `product_cards` entry is one distinct retrieved listing whose `listing_id` is the real listing id, with `title`, `price` and `currency` taken from the indexed listing, at most `top_k` entries, best match first. A listing that was deleted, unpublished (draft or rejected) or never published SHALL NOT be returned. If retrieval fails, the assistant SHALL still answer (an OK response with no cards and a reply that says the catalog could not be searched) and SHALL NOT return any card that was not retrieved. When no RAG store is configured, the assistant keeps answering from its built-in demo catalog.

#### Scenario: A published listing is returned as a product card

- **WHEN** a seller publishes a listing with a unique title and a buyer asks the assistant for that title
- **THEN** the response contains a product card whose `listingId` is that listing's id, with its title and price

#### Scenario: An unpublished listing disappears from the assistant

- **WHEN** the seller of a listing the assistant returns changes it to draft, or deletes another such listing
- **THEN** the assistant no longer returns a card for it

#### Scenario: The assistant answers when retrieval is down

- **WHEN** the embedding backend the RAG store depends on is stopped and a buyer asks the assistant a question
- **THEN** the call succeeds with a non-empty reply and no product cards, and cards come back again once the backend is restored

#### Scenario: Cards are distinct, bounded and ordered by match

- **WHEN** retrieval returns several chunks of one listing and more listings than `top_k`
- **THEN** each listing appears once, at most `top_k` cards are returned, in score order, and hits under `ASSISTANT_RAG_MIN_SCORE` are dropped
- **VERIFIED BY**: team-ai/tests/unit/modules/test_ai_assistant_rag.py › test_cards_are_distinct_bounded_and_filtered_by_score. Not verifiable end to end: chunk counts and scores are not observable through the gateway (design.md).

#### Scenario: Without a RAG store the demo catalog answers

- **WHEN** the service runs with `RAG_ENABLED=false`
- **THEN** the assistant answers from its built-in demo catalog as before
- **VERIFIED BY**: team-ai/tests/unit/modules/test_ai_assistant_rag.py › test_without_rag_store_the_demo_catalog_answers. Not verifiable end to end: the e2e stack runs with the RAG store on (design.md).
