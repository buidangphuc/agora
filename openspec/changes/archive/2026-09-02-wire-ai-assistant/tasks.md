# Tasks

## 1. Code — team-gateway
- [x] `internal/upstream/clients.go`: add `aiv1` import, `AI aiv1.AIServiceClient` field, `aiAddr`
      param to `Dial(...)`, dial + `NewAIServiceClient`.
- [x] `internal/edge/ai.go` (new): `AIForwarder` implementing `aiv1connect.AIServiceHandler`
      (MagicListing / ShoppingAssistant / ChatCopilot via `edge.callRead`) — mirror `listing.go`.
- [x] `internal/edge/server.go`: register `aiv1connect.NewAIServiceHandler(...)` in `NewMux` +
      add `aiv1connect.AIServiceName` to the reflector.
- [x] `internal/config` + `docker-compose.services.yaml`: add `UPSTREAM_AI_ADDR` (team-ai gRPC
      port); wire it into `Dial(...)`.
- [ ] `go build ./...` green (Go not installed locally; runs in Docker build. Generated AI symbol
      names verified against `aiv1connect`/`ai_grpc.pb.go`).

## 2. Code — team-frontend
- [x] `src/lib/gateway/client.ts`: import `AIService`, add `ai: createPromiseClient(AIService, transport)`.
- [x] `src/lib/gateway/ai.ts` (new): wrap `shoppingAssistant / magicListing / chatCopilot`.
- [x] `src/features/assistant/actions.ts` (new): `askAssistantAction`.
- [x] `src/features/listing/actions.ts`: add `magicListingAction` server action.
- [x] `src/features/assistant/AssistantChatView.tsx`: replace `setTimeout` mock with a call to
      `askAssistantAction`; render reply + product cards + follow-ups.
- [x] `src/features/listing/ListingForm.tsx`: "AI Tạo Mô Tả" calls `magicListingAction`.
- [x] Copilot quick-replies in seller chat UI: `src/features/chat/actions.ts` `chatCopilotAction`
      + `ChatView.tsx` "✨ Gợi ý AI" (seller-only) calling team-ai via the gateway; replaced the
      hardcoded suggestions. Verified: ChatCopilot through gateway logs `chat_copilot.replies` in team-ai.
- [x] `tsc --noEmit` green (verified locally, exit 0).

## 3. team-ai
- [x] Start the gRPC server in the FastAPI lifespan when `GRPC_ENABLED=true`
      (`app/bootstrap/application.py`): build via `build_grpc_server`, bind `GRPC_HOST:GRPC_PORT`,
      stop on shutdown. Keep `CHAT_BACKEND=mock`.
- [x] Remove HTTP AI output endpoints: unmount `ai_router` in `app/api/router.py`
      (`/assistant`, `/magic-listing`, `/chat-copilot`). Keep health + completions.
- [x] **Auth (added scope, required):** gRPC `AuthInterceptor` now trusts gateway-forwarded
      `x-principal-{id,type,scopes}` (ADR-0003), with bearer as fallback — previously it required a
      bearer token the gateway does not send, so every gateway→team-ai call would have failed.
- [x] `docker-compose.services.yaml`: team-ai `GRPC_ENABLED=true`, `GRPC_PORT=50060`, expose the
      port; gateway `UPSTREAM_AI_ADDR=team-ai-svc:50060`.

## 4. E2E (platform-e2e)
- [x] Existing AI features already assert the outcome; they now run through the real gateway path
      (no content repoint needed). `make -C platform-e2e features-check` green (team-ai 4/4 automated),
      `make collect` wires the 3 AI tests.
- [x] Ran against a rebuilt parallel stack (frontend :3100 → gateway :8180 → team-ai :50060):
      `test_magic_listing`, `test_assistant`, `test_chat_copilot` → **3 passed**. team-ai logs
      confirm it received `magic_listing.generate` + `shopping_assistant.query` with gateway
      request-ids (real path, not mock).

## 5. Archive
- [x] Verified end-to-end: gateway→team-ai live (Connect JSON), 3 AI e2e green, team-ai gRPC boot
      via lifespan confirmed, gateway `go build` clean in Docker.
- [ ] Archive the change (`/opsx:archive wire-ai-assistant`) — pending your go-ahead.
