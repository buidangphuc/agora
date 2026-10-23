## MODIFIED Requirements

### Requirement: Engagement consumers log a key fingerprint, never the raw key

Every log record emitted by team-engagement Kafka consumers about a consumed record SHALL carry attribute `key_fp`, equal to the first 12 lowercase hex characters of HMAC-SHA256 of the record key bytes under the secret `LOG_KEY_FP_SECRET`, and SHALL NOT carry the raw key in any attribute, message text or error text. An unkeyed SHA-256 prefix of the key SHALL NOT be logged. Message keys, DLQ keys and offset and retry behaviour SHALL be unchanged. In a staging or prod `ENV` the service SHALL refuse to start without `LOG_KEY_FP_SECRET`.

#### Scenario: Engagement key redaction: handler failure logs fingerprint
- **WHEN** the handler fails for a record with key "seller-123" on attempt 1 of 5, and the error text contains "seller-123"
- **THEN** the captured "consume: handler failed" record has `key_fp` equal to the first 12 hex of HMAC-SHA256("seller-123") under the test secret, and the whole captured output contains no occurrence of "seller-123"
- **VERIFIED BY**: team-engagement `internal/events` `TestHandlerFailureLogsKeyFingerprint`

#### Scenario: Engagement key redaction: DLQ parking logs fingerprint
- **WHEN** the handler fails all 5 attempts for key "seller-123" and the DLQ produce succeeds
- **THEN** the "consume: record dead-lettered" record has the same `key_fp`, no `key` attribute, and the DLQ record key still equals the bytes "seller-123"
- **VERIFIED BY**: team-engagement `internal/events` `TestDLQParkLogsFingerprintKeepsDLQKey`

#### Scenario: Engagement key redaction: DLQ produce failure logs fingerprint
- **WHEN** the handler exhausts retries for key "seller-123" and the DLQ produce fails
- **THEN** the "consume: dead-letter failed" record has `key_fp` and no "seller-123", and the consumer returns a non-nil error so the offset does not advance
- **VERIFIED BY**: team-engagement `internal/events` `TestDLQProduceFailureLogsFingerprintOffsetHeld`

#### Scenario: Engagement key redaction: empty key
- **WHEN** the handler fails for a record with a nil key
- **THEN** the log record has `key_fp` equal to the first 12 hex of HMAC-SHA256 of the empty input under the secret (not "e3b0c44298fc"), and no panic occurs
- **VERIFIED BY**: team-engagement `internal/events` `TestEmptyKeyFingerprint`

#### Scenario: Engagement key redaction: fingerprint is exactly 12 hex and stable
- **WHEN** two records with the same key and one record with a different key fail, one of them with a 200-byte key
- **THEN** the first two log records have identical `key_fp`, the third differs, and each `key_fp` matches `^[0-9a-f]{12}$`
- **VERIFIED BY**: team-engagement `internal/events` `TestKeyFingerprintFormatStable`

#### Scenario: Engagement key redaction: fingerprint depends on the secret
- **WHEN** the same key "seller-123" fails under secret A and then under secret B
- **THEN** the two `key_fp` values differ and neither equals the first 12 hex of the plain SHA-256 of "seller-123"
- **VERIFIED BY**: team-engagement `internal/events` `TestKeyFingerprintIsKeyedNotPlainHash`

#### Scenario: Engagement key redaction: strict ENV needs the secret
- **WHEN** `Load` runs with `ENV=prod` and `LOG_KEY_FP_SECRET` empty
- **THEN** it returns an error that names `LOG_KEY_FP_SECRET`
- **VERIFIED BY**: team-engagement `internal/config` `TestStrictEnvRequiresBrokersAuditAndSecret`

#### Scenario: Engagement key redaction: successful records and key unchanged
- **WHEN** the handler succeeds on a record with key "seller-123"
- **THEN** no failure log is written and the handler received the original key bytes "seller-123"
- **VERIFIED BY**: team-engagement `internal/events` `TestSuccessPassesRawKeyToHandler`
