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
