## Why

The Shopping Assistant chatbot returned its reply as a single unary response — the user waited
for the whole answer before seeing anything. team-ai already had a token streamer
(`ChatService.StreamChat` + `ChatStreamer`), but nothing exposed it: the gateway had no streaming
forwarder and the frontend consumed only the unary call. This change wires token streaming
end-to-end so the reply appears progressively (typing effect). LLM stays mock
(`CHAT_BACKEND=mock`), so the stream is a deterministic echo until a real model is enabled.

## What Changes

- **team-gateway** — `ChatForwarder` gains a `StreamChat` server-streaming method that relays
  team-ai's `ChatService.StreamChat` gRPC stream; a second ChatService client (`AIChat`) points at
  team-ai (the existing `Chat` client still points at team-chat for messaging).
- **team-ai** — `ChatServicer.StreamChat` no longer gates on `chat:read` (a scope team-identity
  grants to no role); it is open like the unary `ShoppingAssistant`.
- **team-frontend** — a Next.js route handler `/api/assistant/stream` opens the gateway stream
  server-side and relays deltas as a `ReadableStream`; `AssistantChatView` reads it and fills the
  reply token-by-token, then attaches product cards + follow-ups from the unary call.

## Non-goals

- No real LLM (`CHAT_BACKEND=mock` kept) — streaming a mock is an echo; real streamed replies need
  `llm_router`.
- No streaming for MagicListing or ChatCopilot (MagicListing is unary in the contract; streaming
  it would need an `ai.proto` change).
- No SSE `RealtimeBroker` bridge — Connect server-streaming + a Next.js relay is enough.
