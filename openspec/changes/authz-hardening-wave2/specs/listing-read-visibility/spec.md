## Purpose

Defines who may read listings that are not published and how listing lists are filtered by status in `team-domain`, so drafts and
rejected listings are not readable by anonymous or unrelated callers while owners, admins and internal services keep access.

## ADDED Requirements

### Requirement: Non-published listings are visible only to their owner, admin or a service

`GetListing` SHALL return a listing whose status is not `published` (draft, rejected) only to the principal whose id equals the
listing's `seller_id`, to a principal holding `admin`, or to a principal of type `service` holding `listing.read`. Every other
caller (including anonymous and any other signed-in user) SHALL receive `NOT_FOUND`, identical to an unknown id, with no
listing data. Published listings SHALL remain readable by every caller holding `listing.read`.

#### Scenario: Anonymous caller cannot read a draft

- **WHEN** an anonymous caller calls `GetListing` for a draft listing
- **THEN** the call is `NOT_FOUND`, the same answer as for an unknown id

#### Scenario: Another seller cannot read a draft or rejected listing

- **WHEN** seller B calls `GetListing` for seller A's draft and for seller A's rejected listing
- **THEN** both calls are `NOT_FOUND`

#### Scenario: The owner reads their own draft

- **WHEN** seller A calls `GetListing` for their own draft (the seller edit page)
- **THEN** the listing is returned

#### Scenario: Admin and internal services read non-published listings

- **WHEN** the seeded admin, and a service principal with `listing.read` (promotion or engagement ownership lookup), call `GetListing` for a draft
- **THEN** both receive the listing

#### Scenario: A user principal carrying a service-looking scope set gets no extra access

- **WHEN** a principal of type `user` whose scopes include `listing.read` and `listing.write` but who is not the owner calls `GetListing` for a draft
- **THEN** the call is `NOT_FOUND`

#### Scenario: Published listings stay public

- **WHEN** an anonymous caller calls `GetListing` for a published listing
- **THEN** the listing is returned

### Requirement: Listing lists default to published and restrict other statuses

`ListListings` SHALL treat an empty `status` as `published`. A request for any other status (draft, rejected) SHALL be answered
only for a principal holding `admin` or a principal of type `service` holding `listing.read`; any other caller SHALL receive
`PERMISSION_DENIED` with no listings returned. A seller's own drafts SHALL remain available through `ListMyListings`, which is
unchanged. The request and response shapes SHALL be unchanged.

#### Scenario: An empty status returns only published listings

- **WHEN** an anonymous caller calls `ListListings` with no status after a draft and a rejected listing exist
- **THEN** only published listings are returned and the total counts published listings only

#### Scenario: An explicit published status is unchanged

- **WHEN** a caller calls `ListListings` with status `published`
- **THEN** the result is the same as with an empty status

#### Scenario: A stranger cannot list drafts

- **WHEN** anonymous and buyer callers call `ListListings` with status `draft` and with status `rejected`
- **THEN** every call is `PERMISSION_DENIED`

#### Scenario: Admin may list drafts

- **WHEN** the seeded admin calls `ListListings` with status `draft`
- **THEN** draft listings are returned

#### Scenario: A seller's own drafts remain visible in the seller center

- **WHEN** seller A calls `ListMyListings`
- **THEN** A's draft and published listings are returned
