## ADDED Requirements

### Requirement: Shopping Assistant reply streams token-by-token

The system SHALL stream the assistant's reply text to the browser as it is produced, via
`platform.chat.v1.ChatService/StreamChat` (team-ai's token streamer) routed through the gateway
as a Connect server-stream and relayed to the browser by a Next.js route handler. Product cards
and follow-ups are still resolved via the unary `ShoppingAssistant` call.

#### Scenario: Assistant reply appears progressively

- **WHEN** a buyer sends a message on `/assistant`
- **THEN** the reply text appears incrementally (token deltas) rather than only after the full
  answer is ready, streamed from team-ai through the gateway
