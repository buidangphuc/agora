# consumer-key-redaction Specification

## Purpose
team-engagement consumer logs identify a Kafka record by a 12-hex SHA-256 key fingerprint (key_fp) instead of the raw key, which is a user id.

## Requirements

### Requirement: Engagement consumers log a key fingerprint, never the raw key

Every log record emitted by team-engagement Kafka consumers about a consumed record SHALL carry attribute `key_fp`, equal to the first 12 lowercase hex characters of SHA-256 of the record key bytes, and SHALL NOT carry the raw key in any attribute or message text. Message keys, DLQ keys and offset/retry behaviour SHALL be unchanged.

#### Scenario: Engagement key redaction: handler failure logs fingerprint
- **WHEN** the handler fails for a record with key "seller-123" on attempt 1 of 3
- **THEN** the captured "event handler failed" record has `key_fp` = first 12 hex of SHA-256("seller-123") and the whole captured output contains no occurrence of "seller-123"
- **VERIFIED BY**: team-engagement `internal/consumer` `TestHandlerFailureLogsKeyFingerprint`

#### Scenario: Engagement key redaction: DLQ parking logs fingerprint
- **WHEN** the handler fails all 3 attempts for key "seller-123" and the DLQ produce succeeds
- **THEN** the "event parked to dlq after max attempts" record has the same `key_fp`, `attempts`=3, no `key` attribute, and the DLQ record key still equals the bytes "seller-123"
- **VERIFIED BY**: team-engagement `internal/consumer` `TestDLQParkLogsFingerprintKeepsDLQKey`

#### Scenario: Engagement key redaction: DLQ produce failure logs fingerprint
- **WHEN** the handler exhausts retries for key "seller-123" and the DLQ produce fails
- **THEN** the "dlq produce failed; offset will not advance" record has `key_fp` and no "seller-123", and processRecord returns a non-nil error so the offset does not advance
- **VERIFIED BY**: team-engagement `internal/consumer` `TestDLQProduceFailureLogsFingerprintOffsetHeld`

#### Scenario: Engagement key redaction: empty key
- **WHEN** the handler fails for a record with a nil key
- **THEN** the log record has `key_fp` = "e3b0c44298fc", and no panic occurs
- **VERIFIED BY**: team-engagement `internal/consumer` `TestEmptyKeyFingerprint`

#### Scenario: Engagement key redaction: fingerprint is exactly 12 hex and stable
- **WHEN** two records with the same key and one record with a different key fail
- **THEN** the first two log records have identical `key_fp`, the third differs, and each `key_fp` matches `^[0-9a-f]{12}$` (also for a 200-byte key)
- **VERIFIED BY**: team-engagement `internal/consumer` `TestKeyFingerprintFormatStable`

#### Scenario: Engagement key redaction: successful records and key unchanged
- **WHEN** the handler succeeds on a record with key "seller-123"
- **THEN** no failure log is written and the handler received the original key bytes "seller-123"
- **VERIFIED BY**: team-engagement `internal/consumer` `TestSuccessPassesRawKeyToHandler`
