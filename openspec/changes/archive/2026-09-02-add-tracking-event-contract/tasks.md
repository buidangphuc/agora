# Tasks

## 1. Code — platform-core (proto contract)
- [x] `packages/proto/platform/analytics/v1/analytics.proto` (new): `syntax = "proto3"`,
      `package platform.analytics.v1`. Define `enum EventType { EVENT_TYPE_UNSPECIFIED = 0; VIEW = 1;
      CLICK = 2; ADD_TO_CART = 3; IMPRESSION = 4; }` and
      `message TrackingEvent { EventType event_type = 1; string listing_id = 2; string session_id = 3;
      string anonymous_id = 4; string page_path = 5; string referrer = 6; uint32 position = 7;
      string search_query = 8; map<string,string> properties = 9; }`.
      Doc-comment that it is wrapped in `platform.events.v1.EventEnvelope` and carried on `analytics.events`.
      (Field numbers are the proposed layout; keep zero-value enum suffix relaxed per `buf.yaml`.)
      (Enum values shipped prefixed — `EVENT_TYPE_VIEW/CLICK/ADD_TO_CART/IMPRESSION` — because buf
      `STANDARD` enforces `ENUM_VALUE_PREFIX` (only `ENUM_ZERO_VALUE_SUFFIX` is relaxed); bare `VIEW` etc.
      fail `buf lint` and diverge from every existing enum, e.g. `LISTING_STATUS_*`, `CHANGE_TYPE_*`.)
- [x] `buf lint` clean (run in `packages/proto`). (Ran via `npx @bufbuild/buf lint` — exit 0.)
- [x] `buf breaking` clean against the previous state (additive: new package only, no edits to
      existing `.proto` files). (Ran via `npx @bufbuild/buf breaking --against '.git#branch=main'` — exit 0;
      new file is untracked, no existing `.proto` modified.)
- [x] Document the `analytics.events` topic name alongside `listing.events` in
      `platform-core/docs/DATA_ARCHITECTURE.md` (topic table / CQRS section) — name + payload type only,
      no producer/consumer wiring.

## 2. E2E (platform-e2e)
- [x] **None — contract-only, non-user-facing.** No `FEATURES.yaml` entry and no `.feature` are added
      (the shift-left E2E contract applies to user-facing capability changes). Verification is `buf lint`
      + `buf breaking` + review in track 1. Record this carve-out here so the archive gate is evaluated
      as "no scenarios" rather than "missing scenarios".

## 3. Archive
- [ ] Confirm `buf lint` + `buf breaking` green and the new package imports/compiles when a consumer
      re-vendors it (spot-check with one consumer's `buf generate`, no commit of generated code).
      (`buf lint`/`buf breaking` confirmed green; the consumer `buf generate` spot-check not run — `go`
      not installed locally and it belongs to a consumer repo; runs in CI/Docker.)
- [x] Sync the `tracking` spec delta into `openspec/specs/tracking/spec.md`. (New capability spec created.)
- [x] Archive the change — moved to `changes/archive/2026-09-02-add-tracking-event-contract/`.
      (openspec CLI not installed; archived manually in the established format.)
