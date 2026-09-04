# Tasks

## 1. Code — team-gateway (edge collector + minimal Kafka producer)

- [x] `internal/events/publisher.go` (new): minimal franz-go producer mirroring
      `team-domain/internal/events/publisher.go` — `AnalyticsPublisher` interface with
      `PublishTrackingEvent(ctx, ev *analyticsv1.TrackingEvent, principal *commonv1.Principal, requestID string)`,
      a `KafkaPublisher` (wrap in `EventEnvelope`, `type = "platform.analytics.v1.TrackingEvent"`,
      key = session/anonymous id, `ProduceSync` to `analytics.events`), and a `NoopPublisher`
      for `KAFKA_ENABLED=false`. No business logic — envelope + produce only.
- [x] Vendor + generate the `platform/analytics/v1` proto into team-gateway (`buf generate` in
      the repo tree, per ADR-0001) so `analyticsv1.TrackingEvent` is available. No proto edit.
      — Vendored `proto/platform/analytics/v1/analytics.proto` (copied verbatim from platform-core).
      `buf generate` + go.mod deps (franz-go, google/uuid, kmsg, lz4, klauspost/compress) run in
      Docker/CI (buf + Go not on host); `generated/` is gitignored so codegen happens at build.
- [x] `internal/edge/collector.go` (new): `HandleTrack(w, r)` — parse the beacon JSON (single or
      small batch), reject malformed/unknown type with a 4xx, map to `TrackingEvent`, resolve the
      principal via the edge secret (`Authorization` bearer, falling back to the `session` cookie
      since sendBeacon cannot set Authorization), and call the publisher. Best-effort: a produce
      error is logged, response is 204. No analytics DB, no aggregation.
- [x] `internal/edge/server.go`: register `mux.HandleFunc("POST /api/track", HandleTrack(e, analytics, logger))`
      alongside the existing `/api/events/live` + `/healthz` raw handlers. `NewMux` now takes the
      `events.AnalyticsPublisher`.
- [x] `internal/config/config.go`: added an `Events` group — `KAFKA_ENABLED` (default `false`),
      `KAFKA_BROKERS` (default `localhost:9092`), `KAFKA_ANALYTICS_TOPIC` (default `analytics.events`);
      updated `.env.example` (drift gate). `DeclaredEnvKeys` is reflection-based over Settings groups,
      so it picks up the new keys automatically. Added `KafkaBrokers()` helper.
- [x] Bootstrap wiring: `cmd/gateway/main.go` constructs the publisher (`newAnalyticsPublisher`:
      Kafka when enabled, else no-op) and injects it into `NewMux`; `defer analytics.Close()` on shutdown.
- [ ] `docker-compose.services.yaml`: team-gateway gets `KAFKA_ENABLED=true`,
      `KAFKA_BROKERS=redpanda:9092`, `KAFKA_ANALYTICS_TOPIC=analytics.events`.
      — DEFERRED: `docker-compose.services.yaml` lives at the full_team_repo root, outside the
      team-gateway/team-frontend scope boundary of this work order. Config defaults keep the gateway
      working (no-op) without it; add the three env vars when wiring the compose stack.
- [ ] `go build ./...` + `go vet` + `go test ./...` green (runs in Docker; Go not on host).
      — DEFERRED to CI/Docker: Go is not installed on host and generated proto is gitignored, so the
      module cannot compile locally. Code was written against the real generated symbols
      (`analyticsv1`, `commonv1`, `eventsv1`) and self-reviewed against the neighboring handlers +
      the team-domain publisher. NOTE: a pre-existing `internal/config/config_test.go` references
      `config.Load()` (never defined — only `LoadSettings`); this predates and is unrelated to this
      change, and was left untouched.

## 2. Code — team-frontend (beacon util + call sites)

- [x] `src/lib/track.ts` (new, browser): `track(event: TrackEvent)` posting a small JSON beacon to
      `${NEXT_PUBLIC_GATEWAY_URL}/api/track` via `navigator.sendBeacon`, falling back to
      `fetch(url, { method: "POST", keepalive: true, credentials: "include" })`. Payload = behavioral
      context only (type, listingId, sessionId, anonymousId, path, referrer, position, query,
      properties). Generates/persists an anonymous id (`localStorage`) + session id (`sessionStorage`).
      Never throws into the caller; no-op during SSR. Sent as text/plain to avoid a CORS preflight.
      Added `NEXT_PUBLIC_GATEWAY_URL` to `.env.example`.
- [x] Fire `track({type:"view"})` on PDP mount (`src/features/tracking/TrackView.tsx`, rendered in
      `src/app/listing/[id]/page.tsx`) and `track({type:"click"})` on listing-card click
      (`src/features/tracking/TrackLink.tsx`, used by `src/features/listing/ListingCard.tsx` — covers
      home + search since both render `ListingCard`).
- [x] Fire `track({type:"add_to_cart"})` from the add-to-cart action
      (`src/features/cart/AddToCartButton.tsx`, on success).
- [x] Fire `track({type:"impression"})` for rendered search results with their result position
      (`src/features/tracking/SearchImpressions.tsx`, rendered in `src/app/search/page.tsx`).
- [x] `tsc --noEmit` + unit test for `track.ts` green. `npm run check` (biome + tsc + vitest) GREEN:
      129 files checked, 120 tests pass (incl. 6 new `src/lib/track.test.ts` — beacon shape, no-PII,
      stable ids, no-throw on transport failure). `npm test` GREEN (20 files / 120 tests).
      NOTE: `next build` not run separately (no server build in this environment); `tsc --noEmit`
      passed via `npm run check`.

## 3. E2E (platform-e2e) — user-facing: a browsing action emits an event

- [x] `team-frontend/FEATURES.yaml`: added `tracking.emit-view` (persona buyer, entry `/`,
      services `[team-frontend, team-gateway]`) whose `acceptance` maps 1:1 to the spec scenario
      "A browsing beacon becomes a tracking event on analytics.events". `status: planned` (validated
      against `platform-e2e/schemas/features.schema.json`). It stays `planned` (not `automated`)
      because the e2e scenario is deferred (see below); flip to `automated` + set `covered_by` once
      the platform-e2e feature lands.
- [x] `platform-e2e`: add `.feature` + step defs + helpers driving a browsing action and asserting a
      `TrackingEvent` lands on `analytics.events`. AUTHORED:
      `tests/e2e/features/tracking/emit_tracking.feature` (4 scenarios, names mapped 1:1 to the spec
      `#### Scenario:` titles) + `step_definitions/tracking_steps.py` +
      `step_definitions/test_emit_tracking.py` (binder). New plumbing: `TrackingService`
      (`src/api/services/tracking_service.py`, `POST /api/track` via a raw no-raise `BaseService.send`),
      a lazy-import Kafka consumer helper (`tests/e2e/flows/tracking_flow.py::consume_tracking_events`,
      keeps `--collect-only` free of a kafka client dep), and `KAFKA_BROKERS` / `KAFKA_ANALYTICS_TOPIC`
      settings. Wired into `make collect` (auto-discovered — `pytest --collect-only` shows all 4).
      Reused existing `buyer_steps` ("a buyer is logged in", "a listing has been seeded via the API")
      + `common_steps`. `ruff`/`black` clean; gherkin parses with 0 errors.
- [ ] `make -C platform-e2e features-check` green + flip `tracking.emit-view` to `automated`
      (set `covered_by`); the new scenarios pass against the local stack.
      — GATED ON CI/STACK: the green run needs the full local stack (Go-built gateway + Kafka broker +
      `kafka-python` in the runner), which cannot be built on this host. FEATURES.yaml entry stays
      `status: planned` (no `covered_by`) until then — validated OK by `scripts/features.py`.

## 4. Archive

- [ ] Confirm gateway produces to `analytics.events` end-to-end (browse → topic), frontend beacon
      fires on all four action types, `KAFKA_ENABLED=false` no-op path verified, e2e green.
      — DEFERRED: end-to-end confirmation needs the full stack running (Go build + Kafka + platform-e2e).
- [ ] `openspec archive emit-tracking-events` (folds the tracking delta into `openspec/specs/`).
      — DEFERRED until tracks above are verified in CI/stack.
