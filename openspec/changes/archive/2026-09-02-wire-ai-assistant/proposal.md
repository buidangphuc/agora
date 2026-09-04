## Why

`team-ai` already implements `platform.ai.v1.AIService` (ShoppingAssistant chatbot,
MagicListing "tạo tin nhanh", ChatCopilot) with working mock logic, and the gRPC/Connect
stubs are already generated in `team-gateway` and `team-frontend`. But the capability is not
reachable end-to-end: the gateway has no AI forwarder, the frontend `makeClients()` has no
`ai` client, and the AI UI is faked client-side (`setTimeout`). This change wires the existing
pieces together through the standard path (frontend → gateway → team-ai gRPC) so the AI
features actually run against the backend, using the existing mock model (no API key needed,
no proto change).

## What Changes

- **team-gateway** — add an `AIForwarder` (Connect handler over `platform.ai.v1.AIService`)
  that forwards to team-ai over gRPC; add the `AI` client to the upstream routing table and a
  `UPSTREAM_AI_ADDR` config pointing at team-ai's gRPC port.
- **team-frontend** — register the `ai` client in `makeClients()`, add a gateway data layer +
  Server Actions, and replace the mocked AI UI (assistant chat, magic-listing button) with real
  calls to the gateway.
- **team-ai** — no logic change; keep `CHAT_BACKEND=mock`. Ensure the gRPC AIService container
  is reachable and reads the forwarded `x-principal-*` metadata.
- **E2E** — the three features already exist in `team-ai/FEATURES.yaml`; repoint their coverage
  from the mock UI to the real gateway-backed flow.

## Non-goals

- No change to `platform.ai.v1` proto (contract already exists) — no re-vendor/regenerate.
- No real LLM/RAG yet (`CHAT_BACKEND=llm_router`, Vault-seeded provider key) — a later change.
- No token streaming (SSE `RealtimeBroker` + `StreamChat`) — later; responses are single-shot.
- No new AI capabilities beyond the three already defined.
