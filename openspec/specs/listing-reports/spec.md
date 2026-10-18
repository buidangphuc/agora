# listing-reports Specification

## Purpose
Signed-in users report a listing to team-engagement without the reporter's identity ever being exposed to the reported seller.

## Requirements

### Requirement: A signed-in user reports a listing

`EngagementService.CreateReport` SHALL require an authenticated caller holding `engagement:write`, accept `target_type = LISTING` only, validate `target_id` as a UUID (no existence check), accept a `reason_code` of `prohibited_item`, `counterfeit`, `scam`, `misleading`, `inappropriate` or `other`, and a `note` of at most 500 characters (runes, after trimming). It SHALL store a `Report` with status `OPEN`, `reporter_id` taken from the verified principal (never from the request) and return it. For the same (reporter, listing) while a report is `OPEN` it SHALL return the existing report and create no row.

#### Scenario: Listing reports: Buyer reports a listing
- **WHEN** a signed-in buyer calls CreateReport with a listing UUID, reason `counterfeit` and note "hàng nhái"
- **THEN** the call returns a Report with status `OPEN`, `reporter_id` equal to the caller, `created_at` set, and exactly 1 row exists in `reports`
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportStoresOpenReport`

#### Scenario: Listing reports: Second open report by the same user returns the existing one
- **WHEN** the same user reports the same listing twice while the first report is `OPEN`
- **THEN** both calls return the same report id and `reports` holds 1 row for that pair
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReportIdempotentWhileOpen`

#### Scenario: Listing reports: Concurrent duplicate reports yield one row
- **WHEN** 10 identical CreateReport calls from one user for one listing run concurrently
- **THEN** all 10 return OK with the same report id and `reports` holds exactly 1 row
- **VERIFIED BY**: team-engagement `internal/repository` `TestCreateReportConcurrentDuplicatesOneRow`

#### Scenario: Listing reports: A closed report does not block a new one
- **WHEN** the user's earlier report of a listing was closed `DISMISSED` and the user reports the same listing again
- **THEN** a new report with status `OPEN` and a different id is stored
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReportAfterCloseCreatesNew`

#### Scenario: Listing reports: Different users report the same listing
- **WHEN** two different users each report the same listing
- **THEN** 2 distinct `OPEN` reports exist
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReportTwoReportersTwoRows`

#### Scenario: Listing reports: Anonymous caller is unauthenticated
- **WHEN** CreateReport is called with no principal or the anonymous principal
- **THEN** the call returns `Unauthenticated` and no row is stored
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportAnonymousUnauthenticated`

#### Scenario: Listing reports: Caller without the write scope is denied
- **WHEN** an authenticated caller holding only `engagement:read` calls CreateReport
- **THEN** the call returns `PermissionDenied` and no row is stored
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportMissingWriteScopeDenied`

#### Scenario: Listing reports: Seller cannot report their own listing
- **WHEN** a seller who owns listing L (a `seller_listings` row exists for them and L) calls CreateReport for L
- **THEN** the call returns `FailedPrecondition` and no row is stored
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportOwnListingRefused`

#### Scenario: Listing reports: Unknown reason code is rejected
- **WHEN** a user calls CreateReport with reason code "spam"
- **THEN** the call returns `InvalidArgument` and no row is stored
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportUnknownReasonInvalidArgument`

#### Scenario: Listing reports: Target type and target id are validated
- **WHEN** a user calls CreateReport with target type unspecified, then with `target_id` "not-a-uuid", then with an empty `target_id`
- **THEN** all three calls return `InvalidArgument` and no row is stored
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportTargetValidation`

#### Scenario: Listing reports: Note length boundary
- **WHEN** a user reports one listing with a 500-character note and another listing with a 501-character note
- **THEN** the 500-character call returns OK and the 501-character call returns `InvalidArgument`
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportNoteLengthBoundary`

#### Scenario: Listing reports: Note is optional and trimmed
- **WHEN** a user reports with an empty note, and another user reports with note "  scam  "
- **THEN** both succeed and the stored notes are "" and "scam"
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReportNoteOptionalAndTrimmed`

#### Scenario: Listing reports: Open report cap per reporter
- **WHEN** a user who has 20 `OPEN` reports reports a 21st distinct listing, and then re-reports one of the 20 already open
- **THEN** the 21st call returns `ResourceExhausted` and stores nothing, and the re-report returns the existing report with OK
- **VERIFIED BY**: team-engagement `internal/service` `TestCreateReportOpenCapPerReporter`

#### Scenario: Listing reports: Report of an unknown listing is accepted
- **WHEN** a user reports a well-formed UUID that no listing event has ever mentioned
- **THEN** the call returns OK with status `OPEN` (existence is not checked)
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportUnknownListingAccepted`

#### Scenario: Listing reports: Report note is never logged
- **WHEN** CreateReport runs with note "my phone is 0900000000" and the request fails with an internal error
- **THEN** no log line captured during the call contains the note text
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateReportNoteNotLogged`

#### Scenario: Listing reports: Report created end to end
- **WHEN** a buyer reports a published listing through the gateway with reason `scam`
- **THEN** the gateway returns 200 with a report whose status is `OPEN`, and the same call repeated returns the same report id
- **VERIFIED BY**: platform-e2e `features/engagement/listing_reports.feature` "A buyer reports a listing once"

#### Scenario: Listing reports: Anonymous report is refused end to end
- **WHEN** an anonymous client calls CreateReport through the gateway
- **THEN** the gateway returns HTTP 401 and no report is stored
- **VERIFIED BY**: platform-e2e `features/engagement/listing_reports.feature` "An anonymous visitor cannot report"

#### Scenario: Listing reports: Seller self-report is refused end to end
- **WHEN** a seller reports their own published listing through the gateway
- **THEN** the gateway returns a `failed_precondition` error and no report is stored
- **VERIFIED BY**: platform-e2e `features/engagement/listing_reports.feature` "A seller cannot report their own listing"

### Requirement: Reporter identity is never exposed to the reported seller

No RPC reachable by a non-admin SHALL return another user's `Report`. `ListReports` and `ResolveReport` SHALL be admin-only, and the seller, who has no admin scope, SHALL have no read path to `reporter_id`, `note` or the report status.

#### Scenario: Listing reports: Seller cannot list reports
- **WHEN** the seller of a reported listing, who has no `admin` scope, calls ListReports with that listing id
- **THEN** the call returns `PermissionDenied` (403 at the edge) and the response carries no report
- **VERIFIED BY**: team-engagement `internal/handler` `TestListReportsSellerDenied`

#### Scenario: Listing reports: Seller cannot read reports end to end
- **WHEN** the seller of a reported listing calls ListReports through the gateway
- **THEN** the gateway returns `permission_denied` and the body contains neither the reporter id nor the note
- **VERIFIED BY**: platform-e2e `features/engagement/listing_reports.feature` "The seller cannot see who reported"
