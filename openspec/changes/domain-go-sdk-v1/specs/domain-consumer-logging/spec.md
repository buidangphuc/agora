## MODIFIED Requirements

### Requirement: Consumer logs carry a key fingerprint, not the record key

The team-domain Kafka consumer SHALL log `key_fp`, the first 12 lowercase hex characters of HMAC-SHA256 of the record key keyed by the secret `LOG_KEY_FP_SECRET`, wherever it logs a record (handler failure, DLQ-produce failure, DLQ park), and SHALL NOT write the raw key, in any attribute or message, to any log line. Record keys sent to the DLQ are unchanged. `LOG_KEY_FP_SECRET` SHALL be required when `ENV` is a strict environment and SHALL default to a documented local value otherwise.

#### Scenario: Domain key redaction: Handler failure logs the fingerprint
- **WHEN** a handler fails for a record keyed `user-123` with `LOG_KEY_FP_SECRET=s3cret` and the consumer logs the retry warning
- **THEN** the captured line has `key_fp` = first 12 hex of HMAC-SHA256 key `s3cret` over `user-123` (12 lowercase hex chars) and no attribute named `key`
- **VERIFIED BY**: team-domain `internal/events` `TestIdentityConsumerLogsKeyFingerprint`

#### Scenario: Domain key redaction: Raw key never appears in any log line
- **WHEN** a record keyed `user-123` exhausts its attempts and is parked to the DLQ (retry warnings plus the park warning)
- **THEN** no captured log output contains the substring `user-123`, and the park warning carries the same `key_fp` as the retry warnings
- **VERIFIED BY**: team-domain `internal/events` `TestIdentityConsumerParkLogsNoRawKey`

#### Scenario: Domain key redaction: DLQ produce failure logs the fingerprint
- **WHEN** the DLQ produce fails for a record keyed `user-123` after its handler attempts are exhausted
- **THEN** the consumer run returns a non-nil error, the error log line has `key_fp` and does not contain `user-123`
- **VERIFIED BY**: team-domain `internal/events` `TestIdentityConsumerDLQFailureLogsFingerprint`

#### Scenario: Domain key redaction: Empty key is still fingerprinted
- **WHEN** a handler fails for a record with a nil key under `LOG_KEY_FP_SECRET=s3cret`
- **THEN** the log line has `key_fp` = first 12 hex of HMAC-SHA256 key `s3cret` over empty input, not an empty value
- **VERIFIED BY**: team-domain `internal/events` `TestEmptyKeyIsFingerprinted`

#### Scenario: Domain key redaction: Fingerprint is stable and fixed length
- **WHEN** the fingerprinter is called twice with the same 100-byte key and once with a different key under one secret
- **THEN** both calls for the same key return the same 12-char lowercase hex string and the other key returns a different one
- **VERIFIED BY**: team-domain `internal/events` `TestKeyFingerprintStableTwelveHex`

#### Scenario: Domain key redaction: The fingerprint depends on the secret
- **WHEN** the same key `user-123` is fingerprinted under `LOG_KEY_FP_SECRET=s3cret` and again under `LOG_KEY_FP_SECRET=other`
- **THEN** the two fingerprints differ and neither equals the first 12 hex of the unkeyed SHA-256 of `user-123`
- **VERIFIED BY**: team-domain `internal/events` `TestKeyFingerprintDependsOnSecret`

#### Scenario: Domain key redaction: Kafka key and retry behaviour are unchanged
- **WHEN** a handler fails permanently for a record keyed `user-123`
- **THEN** the DLQ producer receives exactly 1 record whose key is the bytes `user-123` and whose value is unchanged, and the handler ran once
- **VERIFIED BY**: team-domain `internal/events` `TestParkedRecordKeepsOriginalKey`

#### Scenario: Domain key redaction: Successful records log no key
- **WHEN** a handler succeeds on the first attempt for a record keyed `user-123`
- **THEN** the consumer logs no line that contains `user-123` or an attribute `key`
- **VERIFIED BY**: team-domain `internal/events` `TestSuccessfulRecordLogsNoKey`
