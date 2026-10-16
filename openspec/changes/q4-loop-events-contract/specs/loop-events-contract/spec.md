## ADDED Requirements

### Requirement: EventType carries the loop and AI interaction values

`EventType` in `platform/analytics/v1/analytics.proto` SHALL keep values 0-14 unchanged and SHALL add `EVENT_TYPE_SIGN_UP = 15`, `EVENT_TYPE_LOGIN = 16`, `EVENT_TYPE_FOLLOW_SELLER = 17`, `EVENT_TYPE_REFERRAL_REDEEM = 18`, `EVENT_TYPE_DAILY_CHECK_IN = 19`, `EVENT_TYPE_ALERT_SUBSCRIBE = 20` and `EVENT_TYPE_AI_INTERACTION = 21`. The comment on each SHALL name its property keys: `method` (15, 16), `seller_id` (17), none (18; the code is never sent), `streak` optional (19), `alert_type` with `listing_id` set (20), `surface` and `action` allowlisted with no free text (21). No message SHALL change.

#### Scenario: Loop events contract: Loop event types exist with fixed numbers
- **WHEN** the compiled `EventType` descriptor is inspected
- **THEN** the seven names above are values 15 to 21 in that order and values 0 to 14 keep their current names and numbers
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/analytics/v1/loopevents_compat_test.go` (tasks 2.1)

#### Scenario: Loop events contract: Existing values keep their numbers
- **WHEN** the descriptor test compares values 0-14 with the committed expectations (`EVENT_TYPE_SHARE = 10`, `EVENT_TYPE_FAVORITE = 9`, `EVENT_TYPE_PURCHASE = 14`)
- **THEN** all fifteen names and numbers match and no value is removed or renumbered
- **VERIFIED BY**: Go descriptor test `loopevents_compat_test.go` (tasks 2.1)

#### Scenario: Loop events contract: Each new type round-trips
- **WHEN** a `TrackingEvent` with each of `EVENT_TYPE_SIGN_UP` to `EVENT_TYPE_AI_INTERACTION` and an arbitrary properties payload `{surface: assistant, action: shown}` (the descriptor test does not validate it) is serialised and parsed
- **THEN** it parses back with the same `event_type` number and the same properties
- **VERIFIED BY**: Go round-trip test `loopevents_compat_test.go` (tasks 2.1)

#### Scenario: Loop events contract: Event with maximum value round-trips
- **WHEN** a `TrackingEvent` with `event_type = 21` (highest value) and an empty properties map is serialised and parsed
- **THEN** it parses back with `event_type` 21 and no properties
- **VERIFIED BY**: Go round-trip test `loopevents_compat_test.go` (tasks 2.1)

#### Scenario: Loop events contract: Old event bytes still parse
- **WHEN** bytes of a `TrackingEvent` with `event_type = EVENT_TYPE_PURCHASE` produced before this change, and bytes carrying the unknown value 22, are parsed with the new descriptor
- **THEN** both parse without error, the first keeps its fields, and the second exposes numeric value 22 (unknown values preserved)
- **VERIFIED BY**: Go test `loopevents_compat_test.go` (tasks 2.1) plus `make -C platform-core breaking`

#### Scenario: Loop events contract: Property keys are documented
- **WHEN** a reader opens the comments of values 15 to 21
- **THEN** the text contains `method`, `seller_id`, `streak`, `alert_type` with `listing_id` set (value 20), `surface` and `action` on the matching values, and the `EVENT_TYPE_REFERRAL_REDEEM` comment states no properties and that the code is never sent
- **VERIFIED BY**: comment-text assertion in `loopevents_compat_test.go` (tasks 2.1) plus comment review (tasks 1.1)

#### Scenario: Loop events contract: AI event forbids free text
- **WHEN** a reader opens the `EVENT_TYPE_AI_INTERACTION` comment
- **THEN** it states properties are exactly `surface`, `action` (allowlisted) and that no free text (prompt, reply, title, review text) is carried
- **VERIFIED BY**: comment-text assertion in `loopevents_compat_test.go` (tasks 2.1)

#### Scenario: Loop events contract: Missing key comment is caught
- **WHEN** the test's comment-assertion helper runs against a fixture comment for value 19 that lacks `streak`
- **THEN** the helper reports failure (the test expects it), proving a missing key comment is caught
- **VERIFIED BY**: comment-text assertion in `loopevents_compat_test.go` (tasks 2.1)

#### Scenario: Loop events contract: New types in every generated language
- **WHEN** `buf generate` runs on the proto
- **THEN** the Go, Python, JVM and TS outputs each expose the seven new values
- **VERIFIED BY**: `make -C platform-core proto` followed by a grep of each generated tree (tasks 2.2)

#### Scenario: Loop events contract: Change is backward compatible
- **WHEN** `buf breaking --against` the committed HEAD runs on the proto
- **THEN** it exits 0 with no findings
- **VERIFIED BY**: `make -C platform-core breaking` (tasks 1.3)

#### Scenario: Loop events contract: Wire-breaking edit is rejected
- **WHEN** the number or name of any `EventType` value 0-21 is changed or a value is removed
- **THEN** `buf breaking` exits non-zero on a mutated copy that renumbers or removes a value, and the committed descriptor test, which pins all 22 numbers, fails on the same edit
- **VERIFIED BY**: `make -C platform-core breaking` run on a mutated copy plus `loopevents_compat_test.go` (tasks 1.3, 2.1)

#### Scenario: Loop events contract: Lint passes
- **WHEN** `buf lint` runs on the proto
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto` (tasks 1.3)

### Requirement: Analytics proto copies are identical across repos

The updated `analytics.proto` SHALL be copied byte-identical to `proto/platform/analytics/v1/analytics.proto` in team-gateway, team-analytics and team-frontend. Consumers SHALL NOT edit their copy.

#### Scenario: Loop events contract: Copies match the source
- **WHEN** each of the three copies is compared with `platform-core/packages/proto/platform/analytics/v1/analytics.proto`
- **THEN** all three comparisons report identical content (exit 0) and each copy contains the seven new values
- **VERIFIED BY**: one-off slice-done `cmp` of the three copies against the source (tasks 2.3)

#### Scenario: Loop events contract: Drifted or missing copy is rejected
- **WHEN** any consumer copy differs from the source by one byte, or the file is missing
- **THEN** the `cmp` check exits non-zero (a missing file is a failure) and the contract slice is not done
- **VERIFIED BY**: one-off slice-done `cmp` of the three copies against the source (tasks 2.3)

#### Scenario: Loop events contract: Consumer copies lint
- **WHEN** `buf lint` runs in the `proto` directory of each copy owner that has a `buf.yaml`
- **THEN** each of `team-gateway/proto`, `team-analytics/proto` and `team-frontend/proto` exits 0
- **VERIFIED BY**: `buf lint` per consumer proto directory (tasks 2.3)
