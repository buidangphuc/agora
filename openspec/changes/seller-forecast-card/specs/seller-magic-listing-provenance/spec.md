## ADDED Requirements

### Requirement: Magic Listing beacons carry AI provenance
The seller app SHALL map `request_id` and `model_version` from `MagicListingResponse` into the Magic Listing view model and SHALL send them as `impressionId` and `modelVersion` on the item of every `ai_interaction` beacon for that suggestion (`shown`, `accepted`), keeping `properties` exactly `surface` and `action`.

#### Scenario: Shown beacon carries provenance
- **WHEN** the seller generates a Magic Listing suggestion and team-ai returns `request_id` R and `model_version` M
- **THEN** one `ai_interaction` beacon with `properties` `{surface: "magic_listing", action: "shown"}` is queued with `impressionId` R and `modelVersion` M
- **VERIFIED BY**: `team-frontend-seller/src/features/listing/MagicListingAssist.test.tsx` "shown beacon carries request id and model version"

#### Scenario: Accepted beacon reuses the same provenance
- **WHEN** the seller applies a field of the suggestion shown with `request_id` R and `model_version` M
- **THEN** the `accepted` beacon carries `impressionId` R and `modelVersion` M and the listing id when the form has one
- **VERIFIED BY**: `team-frontend-seller/src/features/listing/MagicListingAssist.test.tsx` "accepted beacon reuses the shown provenance"

#### Scenario: Create form has no listing yet
- **WHEN** a suggestion is generated on the create form where no listing id exists
- **THEN** the beacon item has an empty `itemId` and still carries `impressionId` and `modelVersion`
- **VERIFIED BY**: `team-frontend-seller/src/lib/analytics/ai.test.ts` "provenance without a listing id"

#### Scenario: Beacon properties stay allowlisted
- **WHEN** provenance is attached to a beacon
- **THEN** `properties` contains exactly the keys `surface` and `action` and none of the suggestion text, `request_id` or `model_version`
- **VERIFIED BY**: `team-frontend-seller/src/lib/analytics/ai.test.ts` "provenance never enters properties"

#### Scenario: Provenance reaches the warehouse
- **WHEN** a seller generates a Magic Listing suggestion on the stack with team-ai on the mock backend
- **THEN** the stored `ai_interaction` event has `model_version` equal to the response `model_version` and a non-empty `impression_id`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/seller_forecast_card.feature` `Magic Listing beacon carries request id and model version`

### Requirement: Missing provenance is omitted, not invented
The seller app SHALL omit `impressionId` and `modelVersion` from the beacon when `request_id` or `model_version` is an empty string, and SHALL NOT substitute a placeholder or a client-generated id.

#### Scenario: Older team-ai returns no provenance
- **WHEN** `MagicListing` returns empty `request_id` and `model_version`
- **THEN** the beacon has no `impressionId` and no `modelVersion` key and the suggestion is still shown
- **VERIFIED BY**: `team-frontend-seller/src/lib/analytics/ai.test.ts` "empty provenance is omitted"

#### Scenario: Tracker failure never breaks the action
- **WHEN** queuing the beacon throws
- **THEN** the suggestion is still shown and "Áp dụng" still fills the form
- **VERIFIED BY**: `team-frontend-seller/src/features/listing/MagicListingAssist.test.tsx` "tracker failure does not break the assist"

#### Scenario: AI failure sends no beacon
- **WHEN** `magicListingAction` fails
- **THEN** no `shown` beacon is queued and the error Alert with "Thử lại" is shown
- **VERIFIED BY**: `team-frontend-seller/src/features/listing/MagicListingAssist.test.tsx` "failed suggestion sends no beacon"

#### Scenario: Suggestion never overwrites the form on its own
- **WHEN** a suggestion with provenance arrives while the seller has typed their own title
- **THEN** no form field changes until the seller activates "Áp dụng"
- **VERIFIED BY**: `team-frontend-seller/src/features/listing/MagicListingAssist.test.tsx` "never overwrites the form on its own"
