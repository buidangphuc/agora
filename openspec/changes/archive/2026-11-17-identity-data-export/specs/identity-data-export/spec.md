## ADDED Requirements

### Requirement: Export returns the caller's identity-held data

`PrivacyService.ExportMyData` SHALL return, for the calling user principal, one JSON document (`schema_version` 1)
with the keys `account`, `addresses`, `sessions`, `login_history`, `consents`, `deletion` and `truncated`, built only
from data team-identity holds for that user, with `content_type` `application/json; charset=utf-8` and a server-set
`generated_at`. The document SHALL NOT contain the password hash, refresh tokens, handoff codes or `status_reason`.

#### Scenario: E1 Export lists every identity-held record of the caller
- **WHEN** a buyer who has 2 addresses, 3 sessions, 5 login events and 4 consent records calls `ExportMyData`
- **THEN** the response is OK and the JSON holds 2 `addresses`, 3 `sessions`, 5 `login_history` events, 4 `consents` and `account.user_id` equal to the caller's id
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataListsAllRecords`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/data_export.feature` `Export lists every identity-held record of the caller`

#### Scenario: E2 Export never contains credentials
- **WHEN** a user with a password, a live refresh token and an unused handoff code calls `ExportMyData`
- **THEN** the response bytes contain none of the password hash, the refresh token, the handoff code or the `status_reason` text, and the top-level keys are exactly `schema_version`, `generated_at`, `account`, `addresses`, `sessions`, `login_history`, `consents`, `deletion`, `truncated`
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataExcludesCredentialsAndPinsKeys`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/data_export.feature` `Export never contains credentials`

#### Scenario: E3 Consents are listed newest first with deletion state
- **WHEN** a user who accepted Terms at T1, changed analytics consent at T2 later than T1, and requested account deletion calls `ExportMyData`
- **THEN** `consents[0]` is the analytics record recorded at T2, `consents[1]` the Terms record from T1, and `deletion` is `{"pending": true, "erase_after": <request time + 48 h>}`
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataConsentsNewestFirstAndDeletionState`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/data_export.feature` `Consents are listed newest first with deletion state`

#### Scenario: E4 A user with no addresses and no deletion gets empty collections
- **WHEN** a freshly registered user calls `ExportMyData`
- **THEN** `addresses` is `[]`, `deletion` is `{"pending": false, "erase_after": null}` and the response is OK
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataEmptyAccount`

### Requirement: Export is limited to the caller

`ExportMyData` SHALL accept only a USER principal and SHALL take the subject from the principal, never from the
request. An anonymous caller SHALL receive `UNAUTHENTICATED` and a service principal SHALL receive
`PERMISSION_DENIED`, in both cases with no data read.

#### Scenario: E5 Anonymous caller is refused
- **WHEN** a caller without a token calls `ExportMyData` through the gateway
- **THEN** the call fails `UNAUTHENTICATED` and team-identity receives 0 requests
- **VERIFIED BY**: `team-gateway/internal/edge/privacy_export_test.go` `TestExportMyDataAnonymousRefusedAtEdge`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/data_export.feature` `Anonymous caller is refused`

#### Scenario: E6 Service principal is refused
- **WHEN** a service principal with any scope calls `ExportMyData` on team-identity
- **THEN** the call fails `PERMISSION_DENIED` and 0 database reads occur
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataServicePrincipalDenied`

#### Scenario: E7 One user never sees another user's data
- **WHEN** user A and user B each hold distinct addresses and user B calls `ExportMyData`
- **THEN** the document contains only user B's address ids and `account.user_id` equals B's id
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataIsolatedPerUser`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/data_export.feature` `One user never sees another user's data`

#### Scenario: E8 A user with a pending deletion can still export
- **WHEN** a user whose deletion is pending and whose `erase_after` is in the future calls `ExportMyData`
- **THEN** the response is OK and `deletion.pending` is `true`
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataAllowedWhileDeletionPending`

### Requirement: Export is bounded in size and time

The exported `data` SHALL be at most 1 MiB. WHEN `sessions` or `login_history` exceed 1000 rows, THE SYSTEM SHALL
return the newest 1000 and set the matching `truncated` flag. The call SHALL complete within 500 ms at p95 for the
reference account (20 addresses, 200 sessions, 1000 login events).

#### Scenario: E9 Login history over the cap is truncated and flagged
- **WHEN** a user with 1500 login events calls `ExportMyData`
- **THEN** `login_history` holds the newest 1000 events, `truncated.login_history` is `true` and `len(data)` is at most 1048576
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataTruncatesLoginHistory`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/data_export.feature` `Login history over the cap is truncated and flagged`

#### Scenario: E10 Exactly 1000 login events are not truncated
- **WHEN** a user with exactly 1000 login events calls `ExportMyData`
- **THEN** `login_history` holds 1000 events and `truncated.login_history` is `false`
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataCapBoundary`

#### Scenario: E11 Oversized document is shrunk below 1 MiB
- **WHEN** a user whose 1000 login events carry user agents of 2000 bytes each calls `ExportMyData`
- **THEN** the response is OK, `len(data)` is at most 1048576 and `truncated.login_history` is `true`
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataShrinksOversizedDocument`

#### Scenario: E12 Latency for the reference account
- **WHEN** the reference account calls `ExportMyData` 200 times against a Docker Postgres
- **THEN** the p95 of the call duration is at most 500 ms
- **VERIFIED BY**: `team-identity/internal/store/export_pg_test.go` `TestExportMyDataLatencyP95`

### Requirement: Export fails closed and leaves no trace

WHEN any read fails, THE SYSTEM SHALL return an error status and no `data`. The export SHALL NOT be stored by the
server. The edge response SHALL carry `Cache-Control: no-store`. Logs SHALL hold only record counts and byte size.

#### Scenario: E13 A database error returns no partial file
- **WHEN** the login-history read fails with a connection error during `ExportMyData`
- **THEN** the call fails `UNAVAILABLE` and the response `data` is empty
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataDatabaseErrorReturnsNoData`

#### Scenario: E14 The edge response is not cacheable
- **WHEN** a signed-in user calls `ExportMyData` through the gateway
- **THEN** the HTTP response header `Cache-Control` equals `no-store`
- **VERIFIED BY**: `team-gateway/internal/edge/privacy_export_test.go` `TestExportMyDataSetsNoStore`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/identity/data_export.feature` `The edge response is not cacheable`

#### Scenario: E15 Logs carry no personal data
- **WHEN** a user whose username is `export_log_probe` calls `ExportMyData` with log capture enabled
- **THEN** the captured logs contain no user id, no username, no address text and no IP, and contain the byte size and the record counts
- **VERIFIED BY**: `team-identity/internal/rpc/export_test.go` `TestExportMyDataLogsNoPersonalData`
