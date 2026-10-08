## Purpose

Defines the feature registry owned by `team-analytics`: the single machine-readable catalog of every feature
and label definition, the rules for versioning and retiring them, and how consumers discover them. It is the
source both materializations, the dataset builder and the read contract are generated from.

## ADDED Requirements

### Requirement: Every feature is declared once in a machine-readable registry

The system SHALL keep every feature and label definition in one registry file,
`team-analytics/features/registry.yaml`, validated against a published JSON schema. Each entry SHALL declare:
`name`, `version` (positive integer), `kind` (`feature` or `label`), `entity` (`user`, `item`, `user_item`
or `session`), `dtype`, `sql` (one query parameterized only by the as-of time), `source_events` (event types
or topics it reads), `window`, `aggregation`, `ttl` (features only), `owner`, `freshness_slo` (features
only), `pii_class`, `status` (`experimental`, `stable`, `deprecated` or `pending`) and a `description`. The
reference of an entry SHALL be `<entity>.<name>@v<version>` and SHALL be unique. The service SHALL refuse to
start with a registry that does not validate, and the repository's merge gate SHALL fail on it.

#### Scenario: A complete entry validates

- **WHEN** the registry contains an entry `item.views_7d` version 1 with every required field set to an
  allowed value
- **THEN** the registry validator passes and the service loads the entry as `item.views_7d@v1`

#### Scenario: An entry with a missing field fails the merge gate

- **WHEN** a registry entry omits `freshness_slo` or `pii_class`
- **THEN** the validator fails with a message naming the entry and the missing field, and the merge gate
  of `team-analytics` fails

#### Scenario: An unknown entity, dtype or status is rejected

- **WHEN** a registry entry declares entity `seller`, dtype `decimal(9,2)` or status `beta`
- **THEN** the validator fails naming the entry and the invalid value

#### Scenario: Duplicate references are rejected

- **WHEN** two registry entries resolve to the same reference `item.views_7d@v1`
- **THEN** the validator fails naming the duplicated reference

#### Scenario: SQL that does not compile is rejected

- **WHEN** an entry's SQL does not parse, reads a table other than the event tables named in its
  `source_events`, or does not return the entity key, `as_of` and value columns declared for its dtype
- **THEN** the validator fails naming the entry, by compiling the query against an empty copy of the
  warehouse schema

### Requirement: A published feature version is immutable

The system SHALL treat the SQL, entity, dtype, window and aggregation of a registered version as immutable once
that version is on the main branch. Any change that can alter a value SHALL be published as a new version;
the previous version SHALL keep being materialized until it is retired. Changes to `description`, `owner`,
`freshness_slo` and `status` SHALL be allowed in place.

#### Scenario: Editing the SQL of a published version fails

- **WHEN** a change edits the SQL of `item.views_7d@v1`, which already exists on the main branch
- **THEN** the merge gate fails with a message that a value-changing edit needs `item.views_7d@v2`

#### Scenario: A new version is materialized alongside the old one

- **WHEN** `item.views_7d@v2` is added while `item.views_7d@v1` is `stable`
- **THEN** both versions are materialized to both stores, and a consumer pinned to v1 keeps receiving v1
  values

#### Scenario: A metadata-only edit is accepted

- **WHEN** a change edits only the `description` and `owner` of `item.views_7d@v1`
- **THEN** the merge gate passes and the materialized values are unchanged

### Requirement: Features move through a status lifecycle with a sunset date

The system SHALL support the statuses `pending` (declared, its source events do not exist yet, not
materialized), `experimental` (materialized, may be retired without notice period), `stable` (materialized,
covered by the parity gate and the freshness SLO) and `deprecated` (materialized until a declared
`sunset_date` at least 30 days after deprecation). A version SHALL become `stable` only after its parity
check has passed. After its sunset date a deprecated version SHALL stop being materialized and SHALL be
reported as retired by every read.

#### Scenario: A pending feature is declared but not materialized

- **WHEN** `item.orders_30d@v1` is registered with status `pending` because `order.events` does not exist yet
- **THEN** no offline partition and no online field is written for it, and a read reports it as pending

#### Scenario: Deprecation requires a sunset date at least 30 days out

- **WHEN** a change sets a stable version to `deprecated` with a `sunset_date` 10 days away
- **THEN** the validator fails naming the entry and the minimum notice

#### Scenario: A retired version is no longer materialized

- **WHEN** the sunset date of a deprecated version has passed
- **THEN** the materializers stop writing it, and online reads and dataset builds report it as retired
  instead of returning old values

### Requirement: Consumers can discover features through DescribeFeatures

The system SHALL answer `DescribeFeatures` with, for each requested reference (or for all entries when none is
given), the registry metadata, the SQL hash, the status and sunset date, and the last materialized as-of time
per store.

#### Scenario: Describing one feature

- **WHEN** an authorized service calls `DescribeFeatures` for `item.views_7d@v1`
- **THEN** the response carries its entity, dtype, window, TTL, owner, freshness SLO, PII class, status, SQL
  hash and the last offline and online as-of times

#### Scenario: Describing an unknown reference

- **WHEN** an authorized service calls `DescribeFeatures` for `item.views_7d@v9`, which does not exist
- **THEN** the call fails with `NOT_FOUND` naming the reference

### Requirement: Registry changes are owned by team-analytics

The system SHALL require approval from the `team-analytics` owners for every change to the registry, its SQL
or its schema. Any team MAY propose an entry; a proposed entry SHALL start as `experimental` or `pending`.

#### Scenario: A registry change needs an analytics owner review

- **WHEN** a change touches `team-analytics/features/`
- **THEN** the repository's code-owners file names the `team-analytics` owners for that path, and a test of
  the merge gate fails if that entry is removed
