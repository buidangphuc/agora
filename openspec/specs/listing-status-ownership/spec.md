# listing-status-ownership Specification

## Purpose
Listing status is owned by the server and every content write is auto-screened before it can be published.

## Requirements

### Requirement: Listing status is owned by the server

`ListingService.CreateListing` and `UpdateListing`, when called by a seller, SHALL derive the stored status from the state machine and SHALL ignore the client-supplied `status` beyond `DRAFT` and a publish request (`PUBLISHED`). A seller-sent `REJECTED`, `BANNED`, `PENDING_REVIEW`, `HELD` or `UNSPECIFIED` SHALL never be stored and SHALL be treated as "status omitted", which is distinct from `DRAFT`: an omitted status SHALL keep a `PUBLISHED` listing `PUBLISHED` (re-screened on a content change) and a `DRAFT` listing `DRAFT`. A listing stored as `REJECTED` SHALL NOT become `PUBLISHED` by any seller write. A seller write or seller `DeleteListing` on a `BANNED` or `HELD` listing SHALL be refused with `FAILED_PRECONDITION`. An admin's own listing SHALL follow the seller rules (screen included) and SHALL still be audited as `listing.admin_update` on update; an admin editing another seller's listing SHALL keep its stored status. A seller write SHALL apply only if the row's status and `updated_at` are unchanged since the service read it, else `FAILED_PRECONDITION`. `moderation.screen_flags` SHALL be returned only to admins and service principals; the owner SHALL see `reason_code`, `note` and `decided_at`; any other caller SHALL see an empty `moderation`. A seller write to a `REJECTED` listing SHALL move it to `PENDING_REVIEW` (never auto-publish) once the contract slice has added that status; until then (task 1) it SHALL leave it `REJECTED`.

#### Scenario: Status ownership: Seller cannot un-reject by sending PUBLISHED
- **WHEN** a seller calls UpdateListing with `status=PUBLISHED` on their own listing whose stored status is `rejected`
- **THEN** the stored status is not `published` (it is `rejected` after task 1 and `pending_review` once the state machine lands) and `GetListing` by an anonymous caller returns `NotFound`
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventRejectedStaysUnpublished`

#### Scenario: Status ownership: Seller edit of a rejected listing goes to manual review
- **WHEN** a seller calls UpdateListing with any status on their own `rejected` listing
- **THEN** the stored status is `pending_review`, `submitted_at` is set, and the screen verdict does not change that status
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventRejectedGoesToPendingReview`

#### Scenario: Status ownership: Server-only statuses sent on create are ignored
- **WHEN** a seller calls CreateListing with `status` set to `REJECTED`, `BANNED`, `PENDING_REVIEW`, `HELD` or `UNSPECIFIED` in five separate calls
- **THEN** each response carries status `draft` and the stored row status is `draft`
- **VERIFIED BY**: team-domain `internal/handler` `TestCreateListingServerOnlyStatusesStoredAsDraft`

#### Scenario: Status ownership: Seller cannot move a published listing into a server-only status
- **WHEN** a seller calls UpdateListing on their `published` listing with `status=BANNED` and the content is unchanged
- **THEN** the stored status stays `published` and the listing is unchanged
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventClientBannedIgnored`

#### Scenario: Status ownership: Status omitted keeps a published listing published
- **WHEN** a seller calls UpdateListing with `status` unset (`UNSPECIFIED`) changing only the price of their `published` listing
- **THEN** the stored status stays `published` (not `draft`) and one `ListingChanged(UPDATED)` outbox row carries status `published`
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingOmittedStatusKeepsPublished`

#### Scenario: Status ownership: Seller can withdraw a published listing to draft
- **WHEN** a seller calls UpdateListing with `status=DRAFT` on their `published` listing
- **THEN** the stored status is `draft` and one `ListingChanged(UPDATED)` outbox row carries status `draft`
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventPublishedToDraft`

#### Scenario: Status ownership: Banned listing refuses seller writes
- **WHEN** a seller calls UpdateListing on their own listing whose stored status is `banned`
- **THEN** the RPC returns `FailedPrecondition` and the row is unchanged
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingBannedFailedPrecondition`

#### Scenario: Status ownership: Held listing refuses seller writes
- **WHEN** a seller calls UpdateListing on their own listing whose stored status is `held`
- **THEN** the RPC returns `FailedPrecondition` and the row is unchanged
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingHeldFailedPrecondition`

#### Scenario: Status ownership: Banned or held listing refuses seller delete
- **WHEN** the owner calls DeleteListing on their `banned` listing, and in a second case on their `held` listing
- **THEN** each call returns `FailedPrecondition`, the row still exists with its status unchanged and no `ListingChanged(DELETED)` outbox row is written
- **VERIFIED BY**: team-domain `internal/handler` `TestDeleteListingBannedOrHeldFailedPrecondition`

#### Scenario: Status ownership: Seller write racing a moderation decision is refused
- **WHEN** a seller's UpdateListing on their `pending_review` listing reaches the transaction after an admin's ModerateListing `BAN` committed between the seller's read and that transaction
- **THEN** the seller call returns `FailedPrecondition`, the stored status stays `banned` and no outbox row is written for the seller call
- **VERIFIED BY**: team-domain `internal/repository` `TestUpdateTxConditionalOnLoadedStatus`

#### Scenario: Status ownership: Another seller still cannot edit the listing
- **WHEN** a seller who is not the owner and lacks the `admin` scope calls UpdateListing with `status=PUBLISHED` on a `pending_review` listing
- **THEN** the RPC returns `PermissionDenied` and the stored status is `pending_review`
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingNonOwnerNonAdminDeniedNoAudit`

#### Scenario: Status ownership: Admin UpdateListing cannot set status
- **WHEN** an admin calls UpdateListing with `status=PUBLISHED` on a `rejected` listing
- **THEN** the stored status stays `rejected` and the `listing.admin_update` audit event metadata `status` is `rejected`
- **VERIFIED BY**: team-domain `internal/handler` `TestUpdateListingAdminCannotSetStatus`

#### Scenario: Status ownership: Admin content edit does not re-screen
- **WHEN** an admin calls UpdateListing changing the title of another seller's `published` listing
- **THEN** the screen client receives 0 calls and the stored status stays `published`
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventAdminEditNoScreen`

#### Scenario: Status ownership: Admin's own listing goes through the screen
- **WHEN** an admin calls CreateListing with `status=PUBLISHED` for a listing they own whose title contains a prohibited term, then calls UpdateListing changing its description
- **THEN** the create stores `pending_review` with flag `keyword:<term>` (the screen ran), and the update writes exactly 1 `listing.admin_update` audit event
- **VERIFIED BY**: team-domain `internal/service` `TestAdminOwnListingFollowsSellerPath`

#### Scenario: Status ownership: Pending listing is hidden from a stranger
- **WHEN** an anonymous caller calls GetListing on a `pending_review` listing
- **THEN** the RPC returns `NotFound`
- **VERIFIED BY**: team-domain `internal/handler` `TestGetListingPendingReviewHiddenFromStranger`

#### Scenario: Status ownership: Owner sees status and reason but not flags
- **WHEN** the owner calls GetListing on their `rejected` listing that carries reason code `counterfeit`, note `Fake logo` and flag `keyword:replica`
- **THEN** the response has status `rejected`, `moderation.reason_code` `counterfeit`, `moderation.note` `Fake logo` and an empty `moderation.screen_flags`
- **VERIFIED BY**: team-domain `internal/handler` `TestGetListingOwnerSeesReasonNotFlags`

#### Scenario: Status ownership: Admin sees screen flags
- **WHEN** an admin who is not the owner calls GetListing on a `pending_review` listing with flag `keyword:replica`
- **THEN** `moderation.screen_flags` is `["keyword:replica"]`
- **VERIFIED BY**: team-domain `internal/handler` `TestGetListingAdminSeesFlags`

#### Scenario: Status ownership: Screen flags are hidden from non-owners
- **WHEN** an anonymous caller calls GetListing on a `published` listing whose stored `screen_flags` is non-empty
- **THEN** `moderation.screen_flags` is empty and `moderation.note` is empty in the response
- **VERIFIED BY**: team-domain `internal/handler` `TestGetListingStripsModerationForStranger`

### Requirement: Every content write is auto-screened before publish

`CreateListing` and `UpdateListing` by a seller that would store `PUBLISHED` SHALL run the auto-screen on the listing text first: the local prohibited-term list, then `AIService.ScreenListing`. A keyword hit, a `REVIEW` verdict, an error, a timeout, or an unconfigured team-ai SHALL store `PENDING_REVIEW` with the flags recorded in `moderation.screen_flags`. Only an `ALLOW` verdict with no keyword hit SHALL store `PUBLISHED`. Saving a `DRAFT` and editing only price, stock or variants of a `PUBLISHED` listing SHALL NOT call the screen; editing title, description, category or images of a `PUBLISHED` listing SHALL re-screen it whatever status the request sends (`PUBLISHED`, omitted, or a server-only value). The default prohibited-term list SHALL be embedded in the binary; `SCREEN_TERMS_FILE`, when set, SHALL replace it and a missing file SHALL stop start-up.

#### Scenario: Auto-screen: Clean listing publishes
- **WHEN** a seller calls CreateListing with `status=PUBLISHED` and text that has no keyword hit while the screen returns `ALLOW`
- **THEN** the stored status is `published` and one `ListingChanged(CREATED)` outbox row carries status `published`
- **VERIFIED BY**: team-domain `internal/service` `TestCreateWithEventCleanPublishes`

#### Scenario: Auto-screen: Keyword hit goes to review without calling the AI
- **WHEN** a seller calls CreateListing with `status=PUBLISHED` and a title containing a term from the prohibited list
- **THEN** the stored status is `pending_review`, `moderation.screen_flags` is `["keyword:<term>"]` and the AI client receives 0 calls
- **VERIFIED BY**: team-domain `internal/service` `TestCreateWithEventKeywordHitPendingReview`

#### Scenario: Auto-screen: Keyword match ignores case and diacritics
- **WHEN** a seller publishes a listing whose title contains the term `thuốc lá` written as `THUOC LA`
- **THEN** the stored status is `pending_review` with flag `keyword:thuốc lá`
- **VERIFIED BY**: team-domain `internal/screen` `TestKeywordMatchCaseAndDiacriticInsensitive`

#### Scenario: Auto-screen: AI REVIEW verdict goes to review
- **WHEN** a seller publishes a listing with no keyword hit and the screen returns `REVIEW` with label `weapon`
- **THEN** the stored status is `pending_review` and `moderation.screen_flags` is `["ai:weapon"]`
- **VERIFIED BY**: team-domain `internal/service` `TestCreateWithEventAIReviewPendingReview`

#### Scenario: Auto-screen: AI error fails safe
- **WHEN** a seller publishes a listing and the screen call returns an error
- **THEN** the write succeeds with stored status `pending_review` and the listing is not visible to an anonymous GetListing
- **VERIFIED BY**: team-domain `internal/service` `TestCreateWithEventScreenErrorPendingReview`

#### Scenario: Auto-screen: AI timeout fails safe within the bound
- **WHEN** a seller publishes a listing and team-ai does not answer within `UPSTREAM_AI_TIMEOUT_SECONDS` (2 s)
- **THEN** the write returns within 2.5 s with stored status `pending_review`
- **VERIFIED BY**: team-domain `internal/upstream` `TestScreenListingTimeoutBounded` and team-domain `internal/service` `TestCreateWithEventScreenTimeoutPendingReview`

#### Scenario: Auto-screen: Unconfigured team-ai fails safe
- **WHEN** a seller publishes a listing while `UPSTREAM_AI_ADDR` is unset
- **THEN** the stored status is `pending_review`
- **VERIFIED BY**: team-domain `internal/upstream` `TestNewAIClientEmptyAddrIsNil` and team-domain `internal/service` `TestCreateWithEventNilScreenerPendingReview`

#### Scenario: Auto-screen: Draft save is not screened
- **WHEN** a seller calls CreateListing with `status=DRAFT` and a title containing a prohibited term
- **THEN** the stored status is `draft` and the screen client receives 0 calls
- **VERIFIED BY**: team-domain `internal/service` `TestCreateWithEventDraftNotScreened`

#### Scenario: Auto-screen: Price and stock edits do not re-screen
- **WHEN** a seller calls UpdateListing changing only price, stock and variants of their `published` listing
- **THEN** the screen client receives 0 calls and the stored status stays `published`
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventNonContentEditNoScreen`

#### Scenario: Auto-screen: Title edit of a published listing is re-screened
- **WHEN** a seller calls UpdateListing with `status` unset, changing the title of their `published` listing to one containing a prohibited term
- **THEN** the stored status is `pending_review` and one `ListingChanged(UPDATED)` outbox row carries status `pending_review`
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventContentEditRescreens`

#### Scenario: Auto-screen: Content edit sending a server-only status is re-screened
- **WHEN** a seller calls UpdateListing with `status=BANNED`, changing the description of their `published` listing to one containing a prohibited term
- **THEN** the screen runs once and the stored status is `pending_review` (not `published`, not `banned`)
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventServerOnlyStatusContentEditRescreens`

#### Scenario: Auto-screen: Draft published later is screened at that moment
- **WHEN** a seller calls UpdateListing with `status=PUBLISHED` on their `draft` listing whose text matches a prohibited term
- **THEN** the stored status is `pending_review`
- **VERIFIED BY**: team-domain `internal/service` `TestUpdateWithEventDraftToPublishScreens`

#### Scenario: Auto-screen: Screen call uses the service principal
- **WHEN** team-domain calls `ScreenListing` for a seller's publish request
- **THEN** the outgoing metadata holds `x-principal-id` `service-team-domain`, `x-principal-type` `service` and `x-principal-scopes` `ai.classify`, and the seller's principal is not forwarded
- **VERIFIED BY**: team-domain `internal/upstream` `TestScreenListingUsesServicePrincipal`

#### Scenario: Auto-screen: Keyword list loads from the config file
- **WHEN** the service starts with `SCREEN_TERMS_FILE` pointing to a file of 3 terms, one blank line and one `#` comment line
- **THEN** the screener holds exactly 3 terms
- **VERIFIED BY**: team-domain `internal/screen` `TestLoadTermsSkipsBlankAndComments`

#### Scenario: Auto-screen: Missing terms file stops start-up
- **WHEN** `SCREEN_TERMS_FILE` is set to a path that does not exist
- **THEN** start-up returns an error naming the file and the server does not listen
- **VERIFIED BY**: team-domain `internal/config` `TestScreenTermsFileMissingFails`

#### Scenario: Auto-screen: Embedded term list is the default
- **WHEN** the service starts with `SCREEN_TERMS_FILE` unset in the distroless image
- **THEN** start-up succeeds and the screener holds the embedded list (at least 1 term) without reading any file
- **VERIFIED BY**: team-domain `internal/screen` `TestDefaultTermsEmbedded`

#### Scenario: Auto-screen: Publish end to end, clean and flagged
- **WHEN** a seller creates one listing with clean text and one whose title holds the seeded keyword, both with `status=PUBLISHED`, through the gateway
- **THEN** an anonymous GetListing returns the first with status `published` and `NotFound` for the second, while the owner sees the second as `pending_review`
- **VERIFIED BY**: platform-e2e `tests/e2e/features/seller/listing_moderation.feature` "Seller publish goes through the auto-screen"
