## REMOVED Requirements

### Requirement: Shopping Assistant reply streams token-by-token
**Reason**: Decision U9 of docs-ai-data. `/assistant` shows the grounded unary `ShoppingAssistant` answer only. The streamed echo was overwritten by the unary reply and was not grounded in catalog data.
**Migration**: None for users. `ChatService/StreamChat` stays on the gateway and in team-ai for other callers. The Next.js route `/api/assistant/stream` is deleted.
