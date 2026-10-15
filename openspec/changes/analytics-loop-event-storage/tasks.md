Depends on `q4-loop-events-contract` (vendored proto copy with EventType 15-21; run `make proto` in team-analytics before coding).

## Code track (team-analytics)

- [x] 1. Mapping. Write-set: `team-analytics/internal/consumer/tracking.go`, `team-analytics/internal/consumer/tracking_test.go`. Add the seven cases to `eventTypeName`; table test over all 21 values plus an out-of-range value; keep the `unspecified` default. Scenarios: Loop storage: each new type is stored under its name, Loop storage: existing types keep their names, Loop storage: an unrecognised enum value is stored as unspecified without failing, Loop storage: an undecodable payload is still a consumer error.
- [x] 2. Preservation and consent tests. Write-set: `team-analytics/internal/consumer/tracking_test.go`. Scenarios: Loop storage: properties and listing id are preserved, Loop storage: an event with no properties is stored with empty properties, Loop storage: the client id is blanked without analytics consent, Loop storage: a non-tracking envelope is still skipped.
- [x] 3. Docs. Write-set: `team-analytics/README.md` (event-type vocabulary list, seven names). Scenarios: none (documentation).

## E2E track (platform-e2e)

- [x] 4. Feature and bindings. Write-set: `platform-e2e/tests/e2e/features/tracking/loop_event_storage.feature`, `platform-e2e/tests/e2e/step_definitions/test_loop_event_storage.py` (new, `scenarios("tracking/loop_event_storage.feature")`). Build the TrackingEvent payload in the step file (event_type, properties, listing_id, consent), wrap it with `ga4_relay_support.envelope` and send with `ga4_relay_support.produce` (e2e proto stubs regenerate after `q4-loop-events-contract`); read via `tii_support.warehouse_rows` with `settle_s` for the replay case; reuse, do not edit, the shared helpers. Scenario names match the VERIFIED BY lines: "Loop storage: a produced event of each new type reaches the warehouse", "Loop storage: properties and listing id are preserved", "Loop storage: the client id is blanked without analytics consent", "Loop storage: a replayed event is stored once". Scenarios: the four named above.
- [x] 5. `team-analytics/FEATURES.yaml` entries are written by the e2e-author, not by this slice. Scenarios: none.
