## ADDED Requirements

### Requirement: The edge collector accepts the activation and loop beacon types

`POST /api/track` SHALL accept the beacon `type` values `sign_up`, `login`, `follow_seller`, `referral_redeem`, `daily_check_in` and `alert_subscribe` and SHALL publish each as a `TrackingEvent` on `analytics.events` with the matching `EventType` (`EVENT_TYPE_SIGN_UP`, `EVENT_TYPE_LOGIN`, `EVENT_TYPE_FOLLOW_SELLER`, `EVENT_TYPE_REFERRAL_REDEEM`, `EVENT_TYPE_DAILY_CHECK_IN`, `EVENT_TYPE_ALERT_SUBSCRIBE`). The beacon properties SHALL be carried to the event after the existing text scrubbing, and the envelope principal SHALL come from the edge-resolved `session` cookie. Type matching SHALL stay case-insensitive and whitespace-trimmed as for existing types.

#### Scenario: Loop intake: each activation and loop beacon becomes a tracking event
- **WHEN** a visitor posts a beacon of type `sign_up`, `login`, `follow_seller`, `referral_redeem`, `daily_check_in` or `alert_subscribe` (one run per type) with a unique marker in `sessionId`
- **THEN** the response is 202 with accepted 1 and exactly one EventEnvelope whose TrackingEvent has the matching `EventType` and that `sessionId` reaches `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackLoopEventTypes`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: each activation and loop beacon becomes a tracking event"

#### Scenario: Loop intake: properties of a loop beacon reach the event
- **WHEN** a visitor posts a `follow_seller` beacon with properties `seller_id` = "seller-42" and a `daily_check_in` beacon with property `streak` = "7"
- **THEN** the published TrackingEvents carry exactly those properties unchanged
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackLoopEventProperties`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: properties of a loop beacon reach the event"

#### Scenario: Loop intake: the envelope principal is the signed-in buyer
- **WHEN** a signed-in buyer's browser posts a `login` beacon carrying the `session` cookie and a body property `userId` = "someone-else"
- **THEN** the published EventEnvelope principal has type user and the buyer's id, not "someone-else"
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackLoopEventPrincipalFromCookie`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: the envelope principal is the signed-in buyer"

#### Scenario: Loop intake: an anonymous caller is attributed to the anonymous principal
- **WHEN** an anonymous visitor with no `session` cookie posts a `sign_up` beacon
- **THEN** the response is 202 and the published EventEnvelope principal is of type anonymous
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackLoopEventAnonymousPrincipal`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an anonymous caller is attributed to the anonymous principal"

#### Scenario: Loop intake: free text in a loop property is scrubbed
- **WHEN** a visitor posts a `follow_seller` beacon whose `seller_id` property contains an email address
- **THEN** the published property value does not contain the email address
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackLoopEventScrubsProperties`

#### Scenario: Loop intake: an oversized loop property is rejected
- **WHEN** a visitor posts a `referral_redeem` beacon with a property value of 257 characters
- **THEN** the response is 400 and nothing is published to `analytics.events`
- **AND** a `referral_redeem` beacon with a 256-character value, or with 20 property keys, is accepted (202, accepted 1), while one with 21 property keys is answered 400 with nothing published
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackLoopEventBounds`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an oversized loop property is rejected"

#### Scenario: Loop intake: existing beacon types are unchanged
- **WHEN** a visitor posts beacons of type `view`, `share`, `favorite` and the alias `select_item`
- **THEN** they are published as `EVENT_TYPE_VIEW`, `EVENT_TYPE_SHARE`, `EVENT_TYPE_FAVORITE` and `EVENT_TYPE_CLICK`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackExistingTypesUnchanged`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: existing beacon types are unchanged"

#### Scenario: Loop intake: an unknown type is still rejected
- **WHEN** a visitor posts a single beacon of type `teleport`
- **THEN** the response is 400 and nothing is published to `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackUnknownTypeStillRejected`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an unknown type is still rejected"

### Requirement: The edge collector enforces the AI interaction allowlists

`POST /api/track` SHALL accept the beacon `type` `ai_interaction` and publish it with `EventType` `EVENT_TYPE_AI_INTERACTION` only when `properties.surface` is exactly one of `assistant`, `chat_copilot`, `magic_listing`, `review_summary` and `properties.action` is exactly one of `shown`, `accepted`, `edited`, `dismissed`. Matching SHALL be case-sensitive without trimming. A beacon with a missing or out-of-allowlist `surface` or `action` SHALL be dropped; a body with no valid beacon SHALL be answered 400 and nothing SHALL be published. An `ai_interaction` beacon whose `properties` contain any key other than exactly `surface` and `action` SHALL be dropped. Other beacons in a batch SHALL be judged independently (202 with `accepted`/`dropped` counts).

#### Scenario: Loop intake: a valid AI interaction becomes a tracking event
- **WHEN** a visitor posts an `ai_interaction` beacon with surface `assistant`, action `accepted` and `listingId` "lst-1"
- **THEN** one TrackingEvent with `EventType` `EVENT_TYPE_AI_INTERACTION`, listing id "lst-1" and properties surface `assistant` and action `accepted` reaches `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionAccepted`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: a valid AI interaction becomes a tracking event"

#### Scenario: Loop intake: every allowlisted surface and action pair is accepted
- **WHEN** a visitor posts one `ai_interaction` beacon for each of the 4 surfaces crossed with each of the 4 actions, in batches of at most 100 beacons (`maxBatchItems`)
- **THEN** every response is 202 with dropped 0 and the accepted counts sum to 16
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionAllowlistMatrix`

#### Scenario: Loop intake: an AI interaction with a surface outside the allowlist is rejected
- **WHEN** a visitor posts a single `ai_interaction` beacon with surface `search_bar` and action `shown`
- **THEN** the response is 400 and nothing is published to `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionBadSurface`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an AI interaction with a surface outside the allowlist is rejected"

#### Scenario: Loop intake: an AI interaction with an action outside the allowlist is rejected
- **WHEN** a visitor posts a single `ai_interaction` beacon with surface `assistant` and action `typed_a_prompt`
- **THEN** the response is 400 and nothing is published to `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionBadAction`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an AI interaction with an action outside the allowlist is rejected"

#### Scenario: Loop intake: an AI interaction missing surface or action is rejected
- **WHEN** a visitor posts a single `ai_interaction` beacon with no properties, then one with only `surface` set, then one with only `action` set
- **THEN** each response is 400 and nothing is published to `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionMissingProperties`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an AI interaction missing surface or action is rejected"

#### Scenario: Loop intake: AI allowlist matching is exact
- **WHEN** a visitor posts single `ai_interaction` beacons with surface `Assistant`, surface `assistant ` (trailing space) and action `SHOWN`
- **THEN** each response is 400 and nothing is published to `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionExactMatch`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: AI allowlist matching is exact"

#### Scenario: Loop intake: an invalid AI beacon does not drop the valid ones in a batch
- **WHEN** a visitor posts a batch of one valid `ai_interaction` beacon (`review_summary`, `shown`) and one with surface `free_text`
- **THEN** the response is 202 with accepted 1 and dropped 1, and only the valid beacon reaches `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionBatchIndependent`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an invalid AI beacon does not drop the valid ones in a batch"

#### Scenario: Loop intake: an AI interaction is attributed by the cookie not by the body
- **WHEN** an anonymous visitor posts a valid `ai_interaction` beacon
- **THEN** the response is 202 and the published EventEnvelope principal is of type anonymous
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionAnonymous`

#### Scenario: Loop intake: an AI interaction with an extra property key is rejected
- **WHEN** a visitor posts a single `ai_interaction` beacon with surface `assistant`, action `shown` and an additional property `prompt` = "hello"
- **THEN** the response is 400 and nothing is published to `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionExtraKeyRejected`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an AI interaction with an extra property key is rejected"

#### Scenario: Loop intake: an extra-key AI beacon in a batch is dropped and counted
- **WHEN** a visitor posts a batch of one valid `ai_interaction` beacon (`assistant`, `shown`) and one with the same surface and action plus a property `note`
- **THEN** the response is 202 with accepted 1 and dropped 1, and only the valid beacon reaches `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionExtraKeyBatch`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an extra-key AI beacon in a batch is dropped and counted"

#### Scenario: Loop intake: an AI beacon type with odd casing is still validated
- **WHEN** a visitor posts a single beacon of type " AI_Interaction " with surface `search_bar` and action `shown`
- **THEN** the response is 400 and nothing is published to `analytics.events`
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackAIInteractionTypeCasingValidated`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_intake.feature` "Loop intake: an AI beacon type with odd casing is still validated"
