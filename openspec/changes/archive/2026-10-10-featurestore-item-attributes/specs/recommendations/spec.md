## ADDED Requirements

### Requirement: The two-tower stage reads item attributes and user preferences

When the two-tower stage is enabled it SHALL also read the latest `item_attributes@v1` snapshot
(`ITEM_ATTRIBUTES_DIR`, or `ITEM_ATTRIBUTES_PATH`) and the latest `user_preferences@v1` snapshot (`USER_PREFERENCES_DIR`,
or `USER_PREFERENCES_PATH`), and feed the towers each item's category and price and each user's preferred categories.
A listing present only in the attribute snapshot SHALL be in the catalogue with no engagement. The category vocabulary
SHALL be built from the snapshot's categories (most frequent first, at most `TWO_TOWER_MAX_CATEGORIES`). The model's
metadata SHALL record both snapshots (view, version, file, SHA-256) and the vocabulary size. With
`TWO_TOWER_REQUIRE_ATTRIBUTES` true and no item attribute snapshot the run SHALL exit 2 naming `ITEM_ATTRIBUTES_DIR`
before Spark starts and register nothing; without it a missing snapshot leaves category, price and preferences at 0 and
the metadata records no attribute snapshot.

#### Scenario: Cold-start items with different categories get different vectors

- **WHEN** the job runs with the two-tower stage enabled over attribute snapshots that hold two listings with no
  engagement and no ALS factor, with different categories and the same price
- **THEN** both have a non-zero two-tower vector and the vectors differ

#### Scenario: The model records the attribute snapshots it trained on

- **WHEN** the job runs with the two-tower stage enabled over attribute and preference snapshots
- **THEN** the model's metadata records item_attributes@v1 and user_preferences@v1 with their file and SHA-256 and the category vocabulary size

#### Scenario: Required attribute snapshots missing stop the run

- **WHEN** the job runs with the two-tower stage enabled, TWO_TOWER_REQUIRE_ATTRIBUTES true and no attribute snapshots
- **THEN** it exits 2, its log names ITEM_ATTRIBUTES_DIR, and no model is registered

#### Scenario: Without attribute snapshots the stage runs as before

- **WHEN** the job runs with the two-tower stage enabled over popularity and activity snapshots only
- **THEN** it exits 0 and the model's metadata records no attribute snapshot
