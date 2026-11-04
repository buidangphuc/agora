---
"@fe-platform/session": minor
"@fe-platform/api-client": minor
---

Session gains `startHandoff` and `createHandoffHandler` (with `StartHandoffDeps` and `HandoffHandlerDeps`) for the buyer/seller sign-in hand-off. The generated identity client adds `createHandoffCode`, `peekHandoffCode`, `redeemHandoffCode` and `becomeSeller`.
