## ADDED Requirements

### Requirement: Seller can archive own listing

The system SHALL let a seller archive a listing they own.

#### Scenario: Owner archives

- **WHEN** a seller calls ArchiveListing for their own listing
- **THEN** the listing status becomes ARCHIVED and the response is 200

#### Scenario: Unknown listing is rejected

- **WHEN** a seller archives a listing id that does not exist
- **THEN** the response is 404 NOT_FOUND

#### Scenario: Another seller is forbidden

- **WHEN** a seller archives a listing owned by another seller
- **THEN** the response is 403 and the listing status is unchanged
