## ADDED Requirements

### Requirement: The local stack runs the assistant grounded by default

The shipped local compose stack SHALL start team-ai with the RAG store and the listing indexer enabled, so that
`AIService.ShoppingAssistant` answers from indexed listings without any extra overlay. The embedding path (`modelserve-router` and its
embedding backend) SHALL be part of that base stack. A documented overlay SHALL switch RAG off, in which case the assistant
answers from its built-in demo catalog. The service's code default for `RAG_ENABLED` SHALL remain `false`.

#### Scenario: A published listing is returned as a product card

- **WHEN** the stack runs from the base compose (no overlay sets any RAG variable), a seller publishes a listing with a unique title and a buyer asks the assistant for that title
- **THEN** the response contains a product card whose listing id is that listing's id with its title and price

#### Scenario: Without a RAG store the demo catalog answers

- **WHEN** team-ai runs with the RAG-off overlay and a buyer asks the assistant for a t-shirt
- **THEN** every product card is a demo catalog entry and none is a published listing
