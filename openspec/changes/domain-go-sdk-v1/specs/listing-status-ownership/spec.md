## MODIFIED Requirements

### Requirement: Every content write is auto-screened before publish

`CreateListing` and `UpdateListing` by a seller that would store `PUBLISHED` SHALL run the auto-screen on the listing text first: the local prohibited-term list, then `AIService.ScreenListing`. A keyword hit, a `REVIEW` verdict, an error or a timeout SHALL store `PENDING_REVIEW` with the flags recorded in `moderation.screen_flags`. Only an `ALLOW` verdict with no keyword hit SHALL store `PUBLISHED`. Saving a `DRAFT` and editing only price, stock or variants of a `PUBLISHED` listing SHALL NOT call the screen; editing title, description, category or images of a `PUBLISHED` listing SHALL re-screen it whatever status the request sends (`PUBLISHED`, omitted, or a server-only value). The default prohibited-term list SHALL be embedded in the binary; `SCREEN_TERMS_FILE`, when set, SHALL replace it and a missing file SHALL stop start-up. `UPSTREAM_AI_ADDR` is required in every `ENV`, so there is no unconfigured-team-ai mode.

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
- **WHEN** settings load with `ENV=local` and `UPSTREAM_AI_ADDR` unset (everything else valid)
- **THEN** `Load` returns an error naming `UPSTREAM_AI_ADDR`, so the service never starts without team-ai and every publish is screened
- **VERIFIED BY**: team-domain `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

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
