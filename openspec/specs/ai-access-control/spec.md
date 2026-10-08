# ai-access-control Specification

## Purpose
Defines the authorization on AI seller tools, review summaries and recommendations, and how the recommendation
subject is bound to the caller.

## Requirements

### Requirement: AI seller tools need seller rights

`MagicListing` and `ChatCopilot` SHALL require `listing.write`. `ChatCopilot` SHALL refuse a `seller_id` other than
the caller's own, unless the caller is an admin. `SummarizeReviews` SHALL require `listing.read`. Errors SHALL name
only the offending fields or carry a fixed message, never internal detail.

#### Scenario: A buyer cannot use the listing generator

- **WHEN** a logged-in buyer calls `MagicListing` through the gateway
- **THEN** the gateway answers HTTP 403

#### Scenario: A seller cannot run the copilot as another seller

- **WHEN** a logged-in seller calls `ChatCopilot` through the gateway with another seller's `seller_id`
- **THEN** the gateway answers HTTP 403

### Requirement: Recommendations are served for the caller

`Recommend` SHALL serve a signed-in user as themselves, whatever `user_id` the request carries. An anonymous caller
SHALL NOT be able to request recommendations for a `user_id`. An admin or a service MAY request on behalf of a user.

#### Scenario: A buyer cannot fetch another user's recommendations

- **WHEN** a logged-in buyer calls `Recommend` through the gateway with another user's `user_id`
- **THEN** the response is computed for the calling buyer, not for the named user
