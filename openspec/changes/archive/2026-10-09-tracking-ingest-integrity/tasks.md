## 1. Code track (one agent per repo, worktree each)

- [x] 1.1 team-gateway: per-event validation and bounds, 202 with counts / 400 when nothing is valid (D1); deterministic `event_id` from visitor and `eventId` (D2); scrubbing and referrer reduction (D3); verify unit tests for each bound, a mixed batch, the id derivation (same visitor same id, other visitor different id, no `eventId` random), and the scrub table (email, phone forms, 13+ digits, prices kept), then `make check`
- [x] 1.2 team-analytics: `ingested_at` column (DuckDB and BigQuery schema) and the two views (D4, D5); verify DuckDB tests that a redelivered `event_id` writes one row, that `ingested_at` is set, and that the views resolve pre-login rows and anonymous-only rows; then the repo checks
- [x] 1.3 team-frontend: `eventId` on every beacon and one same-id retry of a failed `fetch` flush (D6); verify vitest for the dispatcher (distinct UUIDs, including fan-out) and the queue (one retry, same body), then `npm run check`

## 2. E2E track (platform-e2e; team-gateway, team-analytics and team-frontend FEATURES.yaml)

- [x] 2.1 One FEATURES.yaml acceptance line per scenario in the owning repo, `planned` until merged; verify `features-check`
- [x] 2.2 `tracking/ingest_validation.feature` (mixed batch, oversized properties) and `tracking/ingest_scrubbing.feature` (query masking, referrer), asserted on `analytics.events`; verify they pass
- [x] 2.3 `tracking/ingest_idempotency.feature` (same event twice, two visitors) and `tracking/ingest_time.feature`, asserted in the warehouse (D7); verify they pass
- [x] 2.4 `tracking/identity_stitching.feature` (pre-login resolves, anonymous keeps `anon:`), asserted on `tracking_events_resolved`; verify it passes
- [x] 2.5 `frontend/tracking_event_ids.feature`: a product page's beacons carry distinct UUID `eventId`s (intercepted in the browser); verify it passes
- [x] 2.6 The existing tracking scenarios (`tracking/`, the GA4 data layer, recsys pipeline readers) stay green; the modified collector scenario "A malformed beacon is rejected without producing" still passes

## 3. Review and verify

- [x] 3.1 `contract-boundary-reviewer` (Rule 2 at the collector, broker, analytics ownership) and `auth-scope-reviewer` (the visitor key cannot be spoofed across principals); verify no blocking finding
- [x] 3.2 Gate: `openspec validate tracking-ingest-integrity --strict`, `features.py --strict`, `spec_sync.py tracking-ingest-integrity --strict`, `repo_doctor`
- [x] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane; every new flake root-caused

## Evidence (2026-10-09)

- Code: gateway ded4371b; analytics b7db8686, b45001d8 (ambiguous anonymous ids not stitched); frontend 14e74a4d; pre-existing gofmt drift 03dc2f15.
- e2e: spec_sync 13/13 (11 from the spec plus the review-driven ambiguity scenario; the two modified `tracking` scenarios already existed). Full suite 442/442 (before the ambiguity fix), then on the final code 443/443 and 442/443, plus the destructive lane 48/48 twice. The one failure was the team-ai rate-limit burst, root-caused in 765b33a4.
- Defects and flakes found while integrating, each fixed in its own commit:
  - an unauthenticated `anonymousId` let a logged-in user claim another visitor's anonymous history in `tracking_identity` (boundary review; b45001d8, spec updated);
  - Jaeger's unbounded in-memory store was OOM-killed mid-run (00d6f0bf);
  - team-promotion failed fast on team-domain's old IP after the destructive lane restarted it (WaitForReady, bounded by its call timeout);
  - the rate-limit burst stretched past the sliding window under load (765b33a4).
- Reviews: contract-boundary found no blocking issue. Follow-ups:
  - scrubbing and bounds cover only the fields the spec names (not `coupon`, `itemListName`, `properties` keys, the referrer path);
  - phone masking misses `84…` without `+`;
  - an invalid token on a beacon downgrades to anonymous (intended for beacons, unlike RPCs);
  - the Kafka record key is still the session id (pre-existing).

## 4. Archive

- [x] 4.1 `openspec archive tracking-ingest-integrity`; verify `openspec/specs/tracking` and the new `tracking-ingest-integrity` spec are folded
