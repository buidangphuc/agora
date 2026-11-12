# discovery-followups-contract Specification

## Purpose
TBD - created by archiving change discovery-followups-contract. Update Purpose after archive.

## Requirements

### Requirement: Listing status DELETED value
The listing proto SHALL add `LISTING_STATUS_DELETED = 7` to `ListingStatus`, documented as set by read models for a hard-deleted listing and never returned by team-domain.

#### Scenario: Deleted value present at 7
- **WHEN** the descriptor of `platform.listing.v1.ListingStatus` is inspected
- **THEN** it has a value `LISTING_STATUS_DELETED=7` and exactly 8 values in total
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/followups_compat_test.go` "TestFollowups_ListingStatusDeletedIsSeven"

#### Scenario: Semantics are documented
- **WHEN** the comment on `LISTING_STATUS_DELETED` in listing.proto is read
- **THEN** it states that read models set it for a hard-deleted listing and that team-domain never returns it
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/followups_compat_test.go` "TestFollowups_DeletedStatusDocumented"

#### Scenario: Old consumer meets the new value
- **WHEN** a message carrying a `ListingStatus` field set to 7 is parsed by the previous schema
- **THEN** parsing succeeds, the field reads as enum number 7 with no defined name, and every other field is unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/followups_compat_test.go` "TestFollowups_OldSchemaParsesDeleted"

#### Scenario: Undefined status number is not a defined value
- **WHEN** the descriptor of `ListingStatus` is searched for number 8
- **THEN** no value exists, so the check fails if a value is added without a spec change
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/followups_compat_test.go` "TestFollowups_NoUndefinedStatusNumbers"

### Requirement: Existing listing status values unchanged
`ListingStatus` values 0 to 6 SHALL keep their names and numbers.

#### Scenario: Values 0 to 6 keep their numbers
- **WHEN** the descriptor of `ListingStatus` is compared with the baseline pinned in the test
- **THEN** `LISTING_STATUS_UNSPECIFIED=0`, `DRAFT=1`, `PUBLISHED=2`, `REJECTED=3`, `PENDING_REVIEW=4`, `BANNED=5` and `HELD=6` are identical
- **VERIFIED BY**: `platform-core/packages/proto/platform/listing/v1/followups_compat_test.go` "TestFollowups_ExistingStatusValuesUnchanged"

### Requirement: Listing stock lookup RPC
The search proto SHALL add `rpc GetListingStock(GetListingStockRequest) returns (GetListingStockResponse)` to `SearchService`, with `GetListingStockRequest { repeated string listing_ids = 1; }`, `ListingStock { string listing_id = 1; optional int32 stock = 2; }` and `GetListingStockResponse { repeated ListingStock listings = 1; }`. The comments SHALL state that `listing_ids` holds 1 to 10 ids and any other count is INVALID_ARGUMENT, that the response has one entry per requested id in request order, that `stock` is the listing's projected base stock net of reservations, that a listing is in stock when `stock` is greater than 0 (the same rule as search's `in_stock` filter and `SearchHit.stock`, with no variant rollup), that absent `stock` means not projected or unknown, and that the RPC is service-to-service only.

#### Scenario: RPC and messages present at the specified numbers
- **WHEN** the descriptors of `SearchService`, `GetListingStockRequest`, `ListingStock` and `GetListingStockResponse` are inspected
- **THEN** the service has the `GetListingStock` method from request to response type, the request has `listing_ids=1 repeated string`, `ListingStock` has `listing_id=1 string` and `stock=2 optional int32`, and the response has `listings=1 repeated ListingStock`
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_GetListingStockShape"

#### Scenario: Existing search surface is untouched
- **WHEN** the `SearchService` methods and the messages that existed before this change are compared with the baseline pinned in the test
- **THEN** every earlier rpc, message, field name, number and type is identical
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_ExistingSearchSurfaceUntouched"

#### Scenario: Absent stock is distinct from zero stock
- **WHEN** a `ListingStock` with `stock` set to 0 and a `ListingStock` with `stock` unset are serialised and parsed
- **THEN** the first reads presence true with value 0, the second reads presence false, and the bytes differ
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_StockPresenceRoundTrip"

#### Scenario: Ten-id request round trips at the schema level
- **WHEN** a request with 10 listing ids is serialised and parsed, and a request with no ids is parsed
- **THEN** the first parse equals the original with ids in order, and the second reads an empty list; the schema does not enforce the bound, and rejection of 0 or 11 ids with INVALID_ARGUMENT is verified in the `search-listing-stock-lookup` slice
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_RequestRoundTrip"

#### Scenario: Id bound and ordering are documented
- **WHEN** the comments on `listing_ids` and on `GetListingStockResponse.listings` in search.proto are read
- **THEN** the first contains the text `1..10` and `INVALID_ARGUMENT`, and the second states one entry per requested id in request order
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_BoundAndOrderingDocumented"

#### Scenario: Stock semantics and internal use are documented
- **WHEN** the comments on `ListingStock.stock` and on the `GetListingStock` rpc are read
- **THEN** they state that stock is the projected base stock net of reservations, that in stock means `stock` greater than 0 exactly as the `in_stock` filter and `SearchHit.stock` with no variant rollup, that absent means not projected or unknown, and that the RPC is service-to-service only
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_StockSemanticsDocumented"

#### Scenario: Wire compatible both ways
- **WHEN** bytes of a `SearchService` message serialised by the previous schema are parsed with the new one, and a `ListingStock` is parsed by a schema that lacks the new messages
- **THEN** both parses succeed and existing fields are unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_WireCompatible"

#### Scenario: The RPC carries no personal data
- **WHEN** the fields of the three new messages are listed
- **THEN** they are exactly `listing_ids`, `listing_id`, `stock` and `listings`, with no user, seller, buyer or contact field
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/followups_compat_test.go` "TestFollowups_NoPersonalFields"

### Requirement: Consumer copies stay identical
Every repo that vendors `listing.proto` or `search.proto` under `proto/platform/` SHALL hold a byte-identical copy of the platform-core file after this change, and the contract SHALL pass `buf lint` and `buf breaking` against `origin/main`.

#### Scenario: Copies match the source
- **WHEN** the consumer-copies script compares each vendored `listing.proto` and `search.proto` with the platform-core file
- **THEN** all 15 listing copies and all 14 search copies are byte-identical and the script exits 0
- **VERIFIED BY**: `platform-core/packages/proto/followups_consumer_copies_test.sh` "copies match the source"

#### Scenario: Drifted or missing copy is rejected
- **WHEN** one vendored copy differs by a byte or is missing
- **THEN** the script exits non-zero and names the repo and file
- **VERIFIED BY**: `platform-core/packages/proto/followups_consumer_copies_test.sh` "drifted or missing copy is rejected"

#### Scenario: Renumbering is rejected
- **WHEN** `LISTING_STATUS_DELETED` is changed to a number other than 7, or `listing_ids`, `listing_id`, `stock` or `listings` to another field number
- **THEN** the script or the compat tests fail
- **VERIFIED BY**: `platform-core/packages/proto/followups_consumer_copies_test.sh` "renumbering is rejected"

#### Scenario: Consumer protos still build
- **WHEN** `buf build` runs in every repo holding a refreshed copy
- **THEN** each exits 0
- **VERIFIED BY**: `platform-core/packages/proto/followups_consumer_copies_test.sh` "consumer protos still build"

#### Scenario: Lint and breaking check pass
- **WHEN** `make proto` runs in platform-core
- **THEN** `buf lint` and `buf breaking` against `origin/main` exit 0
- **VERIFIED BY**: `platform-core/packages/proto/followups_consumer_copies_test.sh` "buf lint and breaking pass"
