## MODIFIED Requirements

### Requirement: Logs carry request, principal and trace identifiers

`logx.New(runtime config.Runtime, service string)` SHALL return an `*slog.Logger` using JSON when `LOG_JSON=true` and text otherwise, at the level from `LOG_LEVEL`, with a `service` attribute. Records logged with a context SHALL include `request_id`, `principal_fp` and `trace_id` when the context carries a request id, a principal id and a span. `principal_fp` SHALL be the keyed fingerprint of the principal id under `LOG_KEY_FP_SECRET`; the raw principal id SHALL NOT be logged, under `principal_id` or any other key.

#### Scenario: Context identifiers appear in log records
- **WHEN** a record is logged with a context holding request id `r1`, principal id `u1` and an active span
- **THEN** the JSON record has `request_id=r1`, a non-empty `principal_fp` different from `u1`, the span's `trace_id`, and no `principal_id` key
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/logx` `TestHandlerAddsContextIDs`

#### Scenario: Missing identifiers are omitted, not logged empty
- **WHEN** a record is logged with a context holding no request id, principal or span
- **THEN** the record has no `request_id`, `principal_fp`, `principal_id` or `trace_id` keys
- **VERIFIED BY**: `TestHandlerOmitsMissingIDs`
