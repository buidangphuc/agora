## ADDED Requirements

### Requirement: Order facts record the buyer

Each `order_facts` row SHALL carry `buyer_id`, the `buyer_id` of the `OrderPaidEvent` the row came from, which is the
buyer's user id. All rows of one order SHALL carry the same `buyer_id`. An event with an empty `buyer_id` SHALL be stored
with a NULL `buyer_id`; rows ingested before the column existed SHALL stay in the table with a NULL `buyer_id`. The
Parquet export `order_facts.parquet` SHALL contain the `buyer_id` column.

#### Scenario: A paid order's lines carry the buyer

- **WHEN** a buyer pays an order with two lines of different listings and the next export cycle completes
- **THEN** `order_facts.parquet` on the analytics volume has two rows for that order, each with `buyer_id` equal to
  that buyer's user id
