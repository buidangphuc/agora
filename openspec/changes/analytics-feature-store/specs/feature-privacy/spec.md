## Purpose

Defines the privacy rules every feature store artifact obeys: which kinds of data a feature may carry, how a
user's erasure reaches the online store, the offline store and built datasets, and how long each artifact is
kept. It applies the program rule of heavy but privacy-bounded first-party tracking to derived data.

## ADDED Requirements

### Requirement: Features carry no raw personal data

The system SHALL require every registry entry to declare a `pii_class` of `none`, `pseudonymous_id` or
`behavioral_aggregate`, and SHALL reject an entry whose output column is a direct identifier or free text
(email, phone, name, address, IP address, user agent, raw search query, page path or referrer). Entity keys
SHALL be the internal principal id, `anon:<anonymous_id>` or a listing id only.

#### Scenario: A feature exposing raw search text is rejected

- **WHEN** a registry entry `user.last_query@v1` outputs the raw `search_query` string
- **THEN** the validator fails naming the entry and the forbidden column

#### Scenario: A behavioural aggregate is accepted

- **WHEN** a registry entry outputs a count of searches per user with `pii_class: behavioral_aggregate`
- **THEN** the validator passes

### Requirement: Erasure of a user reaches every feature store artifact

The system SHALL, on an erasure request for a principal (and the anonymous ids stitched to it, once stitching
exists), within 24 hours ensure the principal's raw events are deleted from the warehouse (by the tracking
platform's erasure when it exists, otherwise by the feature store itself), delete the online keys of
that principal for every entity that contains it, exclude the principal from every later offline partition and
dataset, and within 30 days rewrite every retained offline partition and invalidate every retained dataset
that contains it. An invalidated dataset SHALL be refused by the dataset client.

#### Scenario: Online keys disappear within a day

- **WHEN** an erasure request for principal U is processed
- **THEN** `fs:v1:user:U` and every `fs:v1:user_item:U|*` key are gone, and `GetOnlineFeatures` for U reports
  `MISSING`

#### Scenario: Future partitions exclude the erased user

- **WHEN** the batch materializer runs after the erasure of U
- **THEN** no new offline partition contains a row for U

#### Scenario: Retained history is rewritten and datasets invalidated

- **WHEN** offline partitions and a dataset built before the erasure contain rows for U
- **THEN** within 30 days those partitions are rewritten without U, the dataset's manifest is marked
  invalidated, and the dataset client refuses it

### Requirement: Opted-out subjects are excluded from personalization

The system SHALL exclude every subject whose latest consent state recorded by the tracking platform is `denied`
from every feature whose entity is `user`, `user_item` or `session`, and from every row of every dataset built
after the opt-out, and SHALL remove that subject's online values for those entities at the next micro-batch.
Item-level features SHALL keep counting business facts (orders, favorites, reviews) regardless of consent. A
subject that grants consent again SHALL be included from the next run.

#### Scenario: An opted-out user loses personalization features but item facts still count

- **WHEN** user U, who has online `user.recent_items_24h@v1` values and a placed order for listing L, records a
  `denied` consent state
- **THEN** after the next micro-batch `GetOnlineFeatures` for U reports `MISSING`, the next dataset build of
  `als_interactions@v1` has no row for U, and `item.orders_30d@v1` for L still counts U's order

### Requirement: Every feature store artifact has a retention limit

The system SHALL keep raw events under the tracking platform's retention windows, which the feature store
SHALL NOT shorten or enforce itself (behavioural rows 13 months, free text nulled after 90 days, facts 25
months), SHALL apply the same windows to its raw-event backup, and SHALL delete offline partitions older than
180 days and dataset builds older than 30 days unless pinned by a promoted model, which keeps them at most 180
days. Online values SHALL expire through their TTL.

#### Scenario: Old offline partitions are swept

- **WHEN** the retention sweep runs and an offline partition is 181 days old
- **THEN** the partition and its manifest are deleted and the deletion is logged with its prefix

#### Scenario: The feature store does not delete raw events

- **WHEN** the feature-store retention sweep runs over raw events that are 100 days old
- **THEN** those raw rows are untouched by the sweep

#### Scenario: A pinned dataset outlives the default retention

- **WHEN** a dataset build is 45 days old and pinned by a promoted model
- **THEN** the sweep keeps it, and deletes it once it is older than 180 days
