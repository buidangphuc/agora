## Purpose

Defines who can see draft and rejected listings, in listing reads and in search, so that unpublished items are
private to their seller.

## ADDED Requirements

### Requirement: Unpublished listings are private to their owner

`GetListing` SHALL answer `not_found` for a draft or rejected listing unless the caller is its owner, an admin, or a
service holding `listing.read`. `ListListings` SHALL return published listings when no status is given. A request
for draft or rejected listings SHALL require an admin or a service, and anyone else SHALL get `permission_denied`.

#### Scenario: A stranger cannot read a draft

- **WHEN** a buyer calls `GetListing` through the gateway for another seller's draft listing
- **THEN** the gateway answers HTTP 404

#### Scenario: The owner can read their draft

- **WHEN** the seller who owns a draft listing calls `GetListing` for it through the gateway
- **THEN** the listing is returned with status draft

#### Scenario: Listing without a status returns only published listings

- **WHEN** an anonymous client calls `ListListings` through the gateway without a status filter
- **THEN** every returned listing has status published

### Requirement: Search returns published listings unless the seller asks for their own drafts

`SearchListings` and `Suggest` SHALL return only published listings by default, and hits, totals and facets SHALL
agree. A search for drafts SHALL be allowed only for a user whose `seller_id` filter is their own id. Otherwise it
SHALL answer `permission_denied`, or `unauthenticated` for an anonymous caller. Saved-search RPCs SHALL require an
authenticated user.

#### Scenario: Search hides drafts by default

- **WHEN** a seller creates a draft listing with a unique title and an anonymous client searches for that title
- **THEN** the search returns no hit for the draft

#### Scenario: A user cannot search another seller's drafts

- **WHEN** a logged-in buyer searches with status draft and another seller's `seller_id`
- **THEN** the gateway answers HTTP 403

#### Scenario: Saved searches require a signed-in user

- **WHEN** an anonymous client calls `SaveSearch` through the gateway
- **THEN** the gateway answers HTTP 401
