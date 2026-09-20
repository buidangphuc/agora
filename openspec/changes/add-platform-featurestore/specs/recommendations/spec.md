## ADDED Requirements

### Requirement: Unified online/offline feature store

The system SHALL provide a feature store in `platform-featurestore` that serves user and item features online via Redis in under 10ms and reads historical features offline from Parquet snapshots with verifiable training-serving parity.

#### Scenario: Online feature retrieval fetches user and item features

- **WHEN** an inference pipeline requests online features for `user_id="u1"` and `listing_id="item-1"`
- **THEN** the online store returns unified feature vectors within 10ms

#### Scenario: Offline and online feature values exhibit zero parity skew

- **WHEN** features are synced from offline snapshots to the online store
- **THEN** querying online features for any entity returns values identical to the offline dataset
