No dependencies on other slices. No e2e track: log content is verified by unit tests with a captured `slog` handler; the Kafka key and DLQ behaviour are unchanged and already covered by existing identity consumer e2e (no new feature file, no `FEATURES.yaml` entry from this slice).

## Code track (team-domain)

- [x] 1. Fingerprint helper and log change. Write-set: `team-domain/internal/consumer/consumer.go`, new `team-domain/internal/consumer/consumer_log_test.go`. Add `keyFingerprint(key []byte) string` (first 12 hex of SHA-256, `crypto/sha256` + `encoding/hex`); replace the three `slog.String("key", string(key))` calls in `processRecord` with `slog.String("key_fp", keyFingerprint(key))`. Tests use a `slog.NewTextHandler` over a `bytes.Buffer` and the existing `fakeDLQ` from `identity_test.go` (a failing-DLQ variant is added in the new test file). Scenarios: Domain key redaction: Handler failure logs the fingerprint, Domain key redaction: Raw key never appears in any log line, Domain key redaction: DLQ produce failure logs the fingerprint, Domain key redaction: Empty key is still fingerprinted, Domain key redaction: Fingerprint is stable and fixed length, Domain key redaction: Kafka key and retry behaviour are unchanged, Domain key redaction: Successful records log no key.
- [x] 2. README/log docs. Write-set: `team-domain/README.md` only if it documents consumer log fields; otherwise no change. Scenarios: Domain key redaction: Handler failure logs the fingerprint.

## E2E track (platform-e2e)

- [x] 3. None. Every scenario is unit-verified; `team-domain/FEATURES.yaml` is written by the e2e-author if it chooses to list this slice. Scenarios: Domain key redaction: Kafka key and retry behaviour are unchanged (unit only).
