# Tasks

## 1. team-gateway
- [x] `clients.go`: add `AIChat` ChatService client dialing team-ai (aiConn).
- [x] `edge/chat.go`: `ChatForwarder.StreamChat` (Connect server-streaming) relays team-ai's gRPC
      stream; `NewChatForwarder(client, aiChat, edge)`.
- [x] `edge/server.go`: pass `clients.AIChat` to the forwarder.
- [x] Docker `go build` clean (8.9s).

## 2. team-ai
- [x] `servicers/chat.py`: drop the `chat:read` scope gate on `StreamChat` (no role grants it);
      open like the unary assistant.

## 3. team-frontend
- [x] `src/app/api/assistant/stream/route.ts`: server-side relay of the gateway stream as a
      `ReadableStream` (token from cookie).
- [x] `AssistantChatView.tsx`: stream reply text token-by-token, then attach cards + follow-ups
      from the unary call.
- [x] `tsc --noEmit` green.

## 4. Verify
- [x] `curl -N /api/assistant/stream` streams mock echo token-by-token (gateway → team-ai).
- [x] 3 AI e2e still green (no regression).
