## ADDED Requirements

### Requirement: The listing indexer keeps only PUBLISHED listings retrievable

The `listing_indexer` SHALL index a `ListingChanged` event only when its listing status is `PUBLISHED` or legacy `UNSPECIFIED` (unchanged behaviour for producers that do not set a status). For `DRAFT`, `REJECTED`, `PENDING_REVIEW`, `BANNED`, `HELD`, and any status value the decoder does not know, and for the `DELETED` action, it SHALL delete the listing's chunks from the RAG index. Event-id dedupe, stale-event skipping and delete-then-index replacement SHALL be unchanged. `decode.py` SHALL map the three new `ListingStatus` values to their names, SHALL decode `LISTING_STATUS_UNSPECIFIED` (0) to `""` (legacy, indexed), and SHALL decode any other number it does not know to `"UNKNOWN"` (deleted).

#### Scenario: Statuses that hide a listing delete it from the index
- **WHEN** a ListingChanged(UPDATED) event arrives for an indexed listing with status PENDING_REVIEW, BANNED or HELD (one event each, newer each time)
- **THEN** each event deletes the listing from RAG, indexes nothing, and the handler returns "deleted"
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_handler.py` `test_non_published_statuses_delete_from_index`

#### Scenario: A held listing is indexed again when it returns to PUBLISHED
- **WHEN** a HELD event is followed by a newer PUBLISHED event for the same listing
- **THEN** the second event indexes one document with metadata status "PUBLISHED" and the handler returns "indexed"
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_handler.py` `test_held_then_published_is_reindexed`

#### Scenario: A status the decoder does not know is not indexed
- **WHEN** a ListingChanged envelope carries a status number not in the decoder map (for example 99)
- **THEN** the decoded status is "UNKNOWN" and the handler deletes the listing instead of indexing it
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_decode.py` `test_unknown_status_number_decodes_to_unknown` and `tests/unit/modules/listing_indexer/test_handler.py` `test_unknown_status_deletes`

#### Scenario: Decoder maps the new statuses
- **WHEN** envelopes with status PENDING_REVIEW, BANNED and HELD are decoded
- **THEN** the payload statuses are "PENDING_REVIEW", "BANNED" and "HELD"
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_decode.py` `test_new_statuses_are_decoded`

#### Scenario: Status 0 decodes to empty and is indexed
- **WHEN** a ListingChanged envelope whose listing status is 0 (UNSPECIFIED, the producer did not set it) is decoded and handled
- **THEN** the decoded status is "" (not "UNKNOWN") and the handler indexes the listing with metadata status "PUBLISHED"
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_decode.py` `test_unspecified_status_decodes_to_empty` and `tests/unit/modules/listing_indexer/test_handler.py` `test_unspecified_envelope_is_indexed`

#### Scenario: A PUBLISHED listing is still indexed once
- **WHEN** a ListingChanged(CREATED) event with status PUBLISHED is delivered twice with the same event id
- **THEN** the first indexes one document and the second returns "duplicate" without indexing
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_handler.py` `test_repeated_event_id_is_duplicate`

#### Scenario: Legacy UNSPECIFIED status is still indexed
- **WHEN** an event with status UNSPECIFIED (0) arrives
- **THEN** it is indexed with metadata status "PUBLISHED", as before
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_handler.py` `test_unknown_status_defaults_to_published_metadata`

#### Scenario: An older HELD event does not unpublish a newer PUBLISHED listing
- **WHEN** a PUBLISHED event with occurred_at T2 is applied and then a HELD event with occurred_at T1 < T2 for the same listing
- **THEN** the HELD event returns "stale" and the listing stays indexed
- **VERIFIED BY**: team-ai `tests/unit/modules/listing_indexer/test_handler.py` `test_older_event_is_stale`
