# domain-consumer-logging Specification

## Purpose
team-domain consumer logs identify a Kafka record by a 12-hex SHA-256 key fingerprint (key_fp) instead of the raw key, which is a user id.

## Requirements

### Requirement: Consumer logs carry a key fingerprint, not the record key

The team-domain Kafka consumer SHALL log `key_fp`, the first 12 lowercase hex characters of SHA-256 of the record key, wherever it previously logged the record key (handler failure, DLQ-produce failure, DLQ park), and SHALL NOT write the raw key, in any attribute or message, to any log line. Record keys sent to the DLQ are unchanged.

#### Scenario: Domain key redaction: Handler failure logs the fingerprint
- **WHEN** a handler fails for a record keyed `user-123` and `processRecord` logs the retry warning
- **THEN** the captured line has `key_fp` = `SHA-256("user-123")[:12]` (12 lowercase hex chars) and no attribute named `key`
- **VERIFIED BY**: team-domain `internal/consumer` `TestProcessRecordLogsKeyFingerprint`

#### Scenario: Domain key redaction: Raw key never appears in any log line
- **WHEN** a record keyed `user-123` exhausts its attempts and is parked to the DLQ (retry warnings plus the park warning)
- **THEN** no captured log output contains the substring `user-123`, and the park warning carries the same `key_fp` as the retry warnings
- **VERIFIED BY**: team-domain `internal/consumer` `TestProcessRecordParkLogsNoRawKey`

#### Scenario: Domain key redaction: DLQ produce failure logs the fingerprint
- **WHEN** the DLQ produce fails for a record keyed `user-123` after its handler attempts are exhausted
- **THEN** `processRecord` returns a non-nil error, the error log line has `key_fp` and does not contain `user-123`
- **VERIFIED BY**: team-domain `internal/consumer` `TestProcessRecordDLQFailureLogsFingerprint`

#### Scenario: Domain key redaction: Empty key is still fingerprinted
- **WHEN** a handler fails for a record with a nil key
- **THEN** the log line has `key_fp` = `e3b0c44298fc` (SHA-256 of empty input, first 12 hex), not an empty value
- **VERIFIED BY**: team-domain `internal/consumer` `TestKeyFingerprintEmptyKey`

#### Scenario: Domain key redaction: Fingerprint is stable and fixed length
- **WHEN** `keyFingerprint` is called twice with the same 100-byte key and once with a different key
- **THEN** both calls for the same key return the same 12-char lowercase hex string and the other key returns a different one
- **VERIFIED BY**: team-domain `internal/consumer` `TestKeyFingerprintStableAndTwelveHex`

#### Scenario: Domain key redaction: Kafka key and retry behaviour are unchanged
- **WHEN** a handler fails permanently for a record keyed `user-123`
- **THEN** the fake DLQ receives exactly 1 record whose key is the bytes `user-123` and whose value is unchanged, and the handler ran once
- **VERIFIED BY**: team-domain `internal/consumer` `TestProcessRecordDLQKeepsOriginalKey`

#### Scenario: Domain key redaction: Successful records log no key
- **WHEN** a handler succeeds on the first attempt for a record keyed `user-123`
- **THEN** `processRecord` returns nil and the captured log output is empty
- **VERIFIED BY**: team-domain `internal/consumer` `TestProcessRecordSuccessLogsNothing`
