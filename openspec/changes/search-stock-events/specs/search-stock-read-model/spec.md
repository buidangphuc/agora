## Purpose

Defines how the search read-model learns and serves each listing's current stock. It owns the observable
contract of *stock as seen through search*: what number a search reports for a listing, how that number follows
the domain's stock events, and what never happens (regression to an older value, resurrection of a deleted
listing, a stuck partition). It does not decide whether a purchase may proceed; the domain owns that.

## ADDED Requirements

### Requirement: The read-model holds the listing's current stock from stock events

The system SHALL keep, for every indexed listing, an integer stock value derived from `ListingStockChanged`
events, and SHALL update it without re-indexing the listing's other fields. After an accepted event the
indexed stock SHALL equal the event's stock value. A listing that has never received stock information SHALL
report no stock rather than zero.

#### Scenario: A stock event lowers the indexed stock

- **WHEN** a listing is indexed with stock 10 and a `ListingStockChanged` event for it arrives with stock 8
- **THEN** a search that matches the listing reports stock 8 for it, and its title, price and status are
  unchanged

#### Scenario: A stock event raises the indexed stock

- **WHEN** a listing is indexed with stock 3 and a newer `ListingStockChanged` event for it arrives with
  stock 12 (a release or a restock)
- **THEN** a search that matches the listing reports stock 12 for it

#### Scenario: A listing that never received stock information reports none

- **WHEN** a listing was indexed before stock was tracked and no stock event has arrived for it, and no replay has run
- **THEN** its stock is unknown until its next stock event or a replay: a search reports the listing without a stock value (field absent), `in_stock=true` excludes it, and the listing is not treated as out of stock
  by any ranking or display rule of this capability

#### Scenario: An initial stock comes from the listing itself

- **WHEN** a listing is created with stock 7 and no stock event has yet arrived
- **THEN** its indexed stock is 7, and a later stock event for it replaces that value

### Requirement: Stock never regresses to an older value

The system SHALL apply a stock event only if it is newer than the stock state already indexed for that listing,
ordered by the event's occurrence time, independently of any other kind of listing event. A stale, duplicated
or out-of-order stock event SHALL leave the indexed stock unchanged.

#### Scenario: An older stock event arriving late is ignored

- **WHEN** the indexed stock came from an event that occurred at time T2, and a stock event for the same
  listing that occurred at T1 earlier than T2 is then delivered
- **THEN** the indexed stock is unchanged

#### Scenario: A redelivered stock event is idempotent

- **WHEN** the same stock event is delivered twice, or redelivered after a consumer restart
- **THEN** the indexed stock equals the event's value exactly as after the first delivery, and no other field
  of the listing changes

#### Scenario: A newer price or status event does not block a stock event

- **WHEN** the listing's price changed at time T3 and a stock event that occurred at T2 earlier than T3 is
  delivered afterwards for the first time
- **THEN** the stock event is applied, because stock ordering is independent of price and status ordering

#### Scenario: A full listing update does not overwrite newer stock

- **WHEN** a listing has stock 8 from a stock event, and a full `ListingChanged` event for that listing is
  then applied carrying a stale stock value of 10
- **THEN** the indexed stock remains 8

### Requirement: A stock event never creates or resurrects a listing and never blocks progress

The system SHALL ignore a stock event whose listing is not in the index (unknown, not yet indexed, or deleted)
without creating a document, and SHALL treat that as successful handling so the consumer proceeds to the next
record on the partition.

#### Scenario: A stock event for an unknown listing is ignored

- **WHEN** a stock event arrives for a listing id that is not in the index
- **THEN** no document is created, the record is acknowledged, and the next record on the partition is
  processed

#### Scenario: A stock event after a listing is deleted does not resurrect it

- **WHEN** a listing has been deleted from the index and a stock event for it is then delivered
- **THEN** the listing does not appear in search results and no document exists for it

### Requirement: A failure to apply a stock event is retried and then parked, never skipped

The system SHALL apply stock events under the same offset discipline as every other listing event: a failed
application is retried with bounded backoff and, if it still fails, the record is parked on the dead-letter
topic before the consumer advances. A malformed stock event SHALL be handled the same way. The consumer SHALL
NOT advance past a record that has neither been applied nor parked.

#### Scenario: A transient index failure is retried

- **WHEN** applying a stock event fails because the index is briefly unavailable and then recovers
- **THEN** the event is applied on retry, the indexed stock equals the event's value, and the record is
  acknowledged once

#### Scenario: A persistent failure is parked

- **WHEN** applying a stock event keeps failing through every retry
- **THEN** the record is parked on the dead-letter topic and only then does the consumer advance; the failure
  is counted and logged

#### Scenario: A malformed stock event is parked

- **WHEN** a stock event arrives with an empty listing id, a negative stock value, or no occurrence time
- **THEN** it is not applied, it is parked on the dead-letter topic after retries, and the indexed stock of any
  listing is unchanged

### Requirement: Search can report each hit's stock

The search response SHALL carry an optional stock value per hit. The value SHALL be present when the read-model
holds a stock for that listing and absent otherwise. Callers that do not read the field SHALL be unaffected.

#### Scenario: A hit reports the indexed stock

- **WHEN** a buyer searches and a matching listing has indexed stock 5
- **THEN** that hit carries stock 5

#### Scenario: A hit without known stock omits the value

- **WHEN** a matching listing has no indexed stock
- **THEN** that hit carries no stock value, distinguishable from a stock of zero

#### Scenario: An existing caller is unaffected

- **WHEN** a caller built before the field existed searches
- **THEN** it receives the same hits in the same order and ignores the new field

### Requirement: Search can be restricted to listings in stock

The system SHALL accept the structured filter `in_stock=true` and return only listings whose indexed stock is
greater than zero. A listing with no known stock SHALL NOT match the filter. Facet counts and totals SHALL be
computed over the same filtered set as the hits.

#### Scenario: Out-of-stock listings are excluded

- **WHEN** a buyer searches with `in_stock=true` and a matching listing has stock 0
- **THEN** that listing is not in the results and is not counted in totals or facets

#### Scenario: An in-stock listing is included

- **WHEN** a buyer searches with `in_stock=true` and a matching listing has stock 4
- **THEN** that listing is in the results

#### Scenario: A listing with unknown stock does not match

- **WHEN** a buyer searches with `in_stock=true` and a matching listing has no indexed stock
- **THEN** that listing is not in the results

#### Scenario: Omitting the filter changes nothing

- **WHEN** a buyer searches without `in_stock`
- **THEN** in-stock, out-of-stock and unknown-stock listings are all eligible, as before

### Requirement: The stock state survives replay and index rebuild

The system SHALL reach the same stock state whether stock events are applied live or replayed from the event
log into a fresh index, in any interleaving with other listing events for the same listing, and SHALL report
the same stock after a rebuild into a new index generation.

#### Scenario: Replaying the event log reproduces the stock

- **WHEN** the `listing.events` log is replayed from the beginning into an empty index
- **THEN** each listing's indexed stock equals the stock of its newest stock event (or its creation stock if it
  has none)

#### Scenario: A rebuilt index keeps stock

- **WHEN** a new index generation with a different mapping version is built by replaying the event log
- **THEN** every listing's stock is present in the new index with the same value and ordering state
