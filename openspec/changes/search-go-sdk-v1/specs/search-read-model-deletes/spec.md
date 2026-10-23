## MODIFIED Requirements

### Requirement: Tombstones are retained for a configurable time, then purged

The search indexer SHALL remove tombstones older than `TOMBSTONE_TTL` (default 14 days), measured from when the
indexer applied the delete, checking every `TOMBSTONE_PURGE_INTERVAL` (default 1 hour). Both are Go durations. An
unparsable or non-positive value SHALL make the indexer refuse to start with a message naming the key; an unset value
SHALL take the default. Purging SHALL remove only tombstones, never a live document.

#### Scenario: An expired tombstone is purged

- **GIVEN** the e2e stack runs the search indexer with a short `TOMBSTONE_TTL` and `TOMBSTONE_PURGE_INTERVAL`
- **WHEN** a seller deletes one of two published listings through the gateway
- **THEN** the read-model holds a tombstone for the deleted listing right after the delete is consumed, holds no document for it once the TTL plus one purge interval has passed, and the other listing still appears in `SearchListings`
- **VERIFIED BY**: `platform-e2e` `search/search_read_model_deletes.feature` "An expired tombstone is purged", and `team-search/internal/tombstone` `TestPurge_UsesTTLCutoff` and `team-search/internal/index` `TestIT_PurgeTombstones_RemovesOnlyExpiredTombstones`

#### Scenario: An invalid tombstone duration refuses boot

- **WHEN** the indexer settings are loaded with `TOMBSTONE_TTL=soon` or `TOMBSTONE_PURGE_INTERVAL=0s`
- **THEN** `LoadIndexer` returns an error that names the offending key
- **VERIFIED BY**: `team-search/internal/config` `TestTombstoneDurationsRefuseInvalid`

#### Scenario: Unset tombstone durations take the defaults

- **WHEN** the indexer settings are loaded with neither `TOMBSTONE_TTL` nor `TOMBSTONE_PURGE_INTERVAL` set
- **THEN** the TTL is 336h and the purge interval is 1h
- **VERIFIED BY**: `team-search/internal/config` `TestTombstoneDefaults`
