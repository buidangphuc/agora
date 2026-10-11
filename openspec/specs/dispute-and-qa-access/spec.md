# dispute-and-qa-access Specification

## Purpose
Defines who may open, read and resolve disputes, and when an answer is marked as a shop reply.

## Requirements

### Requirement: Disputes are tied to a real order and its parties

`CreateDispute` SHALL look up the order. It SHALL answer `not_found` when the order does not exist or the caller is
not its buyer. It SHALL answer `permission_denied` when the named defendant is not the order's seller.
`GetDispute` SHALL return the dispute only to its claimant, its defendant or an admin, and answer `not_found` to
anyone else. `ResolveDispute` SHALL require an admin.

#### Scenario: The buyer opens a dispute against the order's seller

- **WHEN** the buyer of an order opens a dispute naming the order's seller
- **THEN** the dispute is created and both the buyer and the seller can read it

#### Scenario: A buyer cannot name a different defendant

- **WHEN** the buyer of an order opens a dispute naming a user who is not the order's seller
- **THEN** the gateway answers HTTP 403

#### Scenario: A stranger cannot open a dispute on someone else's order

- **WHEN** a buyer who did not place the order opens a dispute on it
- **THEN** the gateway answers HTTP 404

#### Scenario: A stranger cannot read a dispute

- **WHEN** a logged-in user who is neither party nor admin calls `GetDispute` for an existing dispute
- **THEN** the gateway answers HTTP 404

### Requirement: Shop replies are attributed only to the listing owner

An answer SHALL be stored as a shop reply only when the caller owns the question's listing. A request with
`is_shop_reply` from anyone else SHALL be stored as an ordinary answer.

#### Scenario: A non-owner cannot post a shop reply

- **WHEN** a buyer answers a question on another seller's listing with `is_shop_reply` set
- **THEN** the stored answer is not marked as a shop reply
