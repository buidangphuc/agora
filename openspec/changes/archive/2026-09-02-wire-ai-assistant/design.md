## Context

`platform.ai.v1.AIService` and `team-ai`'s implementation already exist; the gRPC/Connect stubs
are generated in gateway and frontend. This is a wiring change following architecture rules 1–2
(frontend talks only to the gateway; the gateway is a pure router forwarding `x-principal-*`).

## Decisions

- **Transport: gRPC, not HTTP.** The gateway dials team-ai's gRPC `AIService` (matching the
  already-generated `aiv1connect` stubs and every other upstream), not the FastAPI HTTP port
  `:8000`. `UPSTREAM_AI_ADDR` points at team-ai's gRPC listen port.
- **team-ai must actually serve gRPC at runtime.** Discovered during apply: the container CMD is
  `uvicorn main:app` and the lifespan does NOT start the gRPC server (`build_grpc_server`/`serve`
  exist but only tests use them). So team-ai currently serves HTTP only. This change starts the
  gRPC server inside the FastAPI lifespan when `GRPC_ENABLED=true`
  (`rag_provider=lambda: app.state.resources.rag_service`, `chat_streamer=build_chat_streamer`),
  binds `GRPC_HOST:GRPC_PORT`, and stops it on shutdown; compose exposes the gRPC port.
- **Remove team-ai's HTTP AI output endpoints.** Per the decision, drop the `/assistant`,
  `/magic-listing`, `/chat-copilot` FastAPI routes (unmount `ai_router` in `app/api/router.py`)
  so the AI capability has exactly one path — through the gateway over gRPC — and the frontend
  cannot reach team-ai directly. Health + completions HTTP routes stay (Docker healthcheck uses
  HTTP `/healthz`).
- **Reuse the ListingForwarder pattern.** `AIForwarder` copies
  `team-gateway/internal/edge/listing.go`: `edge.callRead` for all three RPCs (read-only, no
  Kafka emit), `edge.outgoing(ctx, header)`, `toConnectErr`. No business logic in the gateway.
- **Auth via forwarded principal (ADR-0003).** Gateway verifies JWT once and forwards
  `x-principal-*`. Discovered during apply: team-ai's gRPC `AuthInterceptor` only accepted a
  bearer token, which the gateway does not send — so it now trusts the forwarded
  `x-principal-{id,type,scopes}` (bearer kept as a direct-call fallback). The `AIServicer` does
  not gate scopes, so a present principal is sufficient.
- **Model stays mock.** `CHAT_BACKEND=mock` — the LLM/RAG seam is untouched; enabling real
  models later is an env change in team-ai, invisible to gateway/frontend.
- **Frontend server-only.** The `ai` client is built in `makeClients()` (bearer from cookie) and
  called from Server Actions / RSC; the browser never calls team-ai.

## Risks / Trade-offs

- team-ai's `GRPC_PORT` default (50051) collides conceptually with team-domain's; confirm the
  actual port team-ai listens on in `docker-compose.services.yaml` before setting `UPSTREAM_AI_ADDR`.
- Mock responses are deterministic and not truly "AI"; acceptable for wiring verification, called
  out in Non-goals.
