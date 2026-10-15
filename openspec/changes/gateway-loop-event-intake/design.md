## Clarifications

### Round 1 — 2026-10-15

- Human decision: an `ai_interaction` beacon whose `properties` contain any key other than exactly `surface` and `action` is rejected. A single such beacon is answered 400 and nothing is published; inside a batch it is dropped and counted in `dropped`. Why: keeps free text out of the warehouse at the edge even under an extra key. `listingId` is a top-level beacon field, not a property, so it is unaffected.
- Principal source: the envelope principal always comes from the `session` cookie; a `userId` in the beacon body or properties is ignored.
- Retention: no explicit analytics.events retention policy was found in the repo docs; this slice does not change it.
