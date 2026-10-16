Paths relative to the repo root. E2E track: none (pure contract; behaviour is e2e-tested in the gateway, analytics and frontend slices). FEATURES.yaml entries for platform-core are `not-testable` with reasons (done by the e2e-author).

## 1. Code track — proto
Write-set: `platform-core/packages/proto/platform/analytics/v1/analytics.proto`
- [x] 1.1 Append `EVENT_TYPE_SIGN_UP = 15` .. `EVENT_TYPE_AI_INTERACTION = 21` to `EventType` with the property-key comments from the initiative Contracts (covers: Loop events contract: Loop event types exist with fixed numbers, Loop events contract: Property keys are documented, Loop events contract: AI event forbids free text).
- [x] 1.2 Leave values 0-14 and every message untouched (covers: Loop events contract: Existing values keep their numbers, Loop events contract: Old event bytes still parse).
- [x] 1.3 Run `make -C platform-core lint-proto breaking` before the slice commit, against main / the stacked base (covers: Loop events contract: Change is backward compatible, Loop events contract: Wire-breaking edit is rejected, Loop events contract: Lint passes).

## 2. Verification, generation and copies
Write-set: `platform-core/packages/proto/platform/analytics/v1/loopevents_compat_test.go`, `{team-gateway,team-analytics,team-frontend}/proto/platform/analytics/v1/analytics.proto`
- [x] 2.1 Write the Go test first (red), mirroring `consent_compat_test.go`: enum numbers and names 0-21, round-trip of each new type, unknown-value tolerance, old-bytes parse, comment assertions (incl. `listing_id` on value 20, REFERRAL_REDEEM states no properties, value 21 "exactly surface, action", and a fixture comment lacking `streak` that the helper must reject); then green via 1.x (covers: Loop events contract: Loop event types exist with fixed numbers, Loop events contract: Each new type round-trips, Loop events contract: Event with maximum value round-trips, Loop events contract: Old event bytes still parse, Loop events contract: Existing values keep their numbers, Loop events contract: Property keys are documented, Loop events contract: AI event forbids free text, Loop events contract: Missing key comment is caught).
- [x] 2.2 Run `make -C platform-core proto`; confirm Go, Python, JVM and TS outputs expose the seven values (covers: Loop events contract: New types in every generated language).
- [x] 2.3 Copy `analytics.proto` byte-identical into the three trees; run `cmp` against the source as a one-off slice-done check (missing file = failure) and `buf lint` in each proto dir with a `buf.yaml` (covers: Loop events contract: Copies match the source, Loop events contract: Drifted or missing copy is rejected, Loop events contract: Consumer copies lint).

## 3. E2E track
- [x] 3.1 None: no user-facing behaviour. `platform-core/FEATURES.yaml` entries with `status: not-testable` (reason: pure contract, verified by the Go test, `buf breaking` and `cmp`) for every scenario (done by the e2e-author).
