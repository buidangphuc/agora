## Purpose

Defines who may create, read and resolve disputes and how product answers are attributed to the shop in `team-engagement`, so a dispute can only be opened by the order's buyer against its seller, read by its parties, and decided by an admin.

## ADDED Requirements

### Requirement: Only an admin resolves disputes

`team-engagement` SHALL require the scope `admin` on `ResolveDispute`. A signed-in caller without `admin` (buyer, seller)
SHALL receive `PERMISSION_DENIED` and the dispute SHALL be unchanged; a caller without a principal or with the anonymous
principal SHALL receive `UNAUTHENTICATED`. The existing terminal-state rule is unchanged (a `RESOLVED` or `REJECTED`
dispute answers `FAILED_PRECONDITION`).

#### Scenario: Buyer cannot resolve a dispute

- **WHEN** the buyer who opened a dispute calls `ResolveDispute` with status `RESOLVED`
- **THEN** the call is `PERMISSION_DENIED` and the dispute is still `OPEN`

#### Scenario: Seller cannot resolve a dispute against themselves

- **WHEN** the defendant seller calls `ResolveDispute` with status `REJECTED`
- **THEN** the call is `PERMISSION_DENIED` and the dispute is still `OPEN`

#### Scenario: Seeded admin resolves a dispute

- **WHEN** the seeded admin (logged in as `admin`) calls `ResolveDispute` with status `RESOLVED` and a resolution
- **THEN** the dispute is `RESOLVED` with that resolution

#### Scenario: A user who registers as "admin" is not an admin

- **WHEN** a new account is registered with role `admin` and calls `ResolveDispute`
- **THEN** the call is `PERMISSION_DENIED` (registration gives a buyer)

### Requirement: Disputes are readable only by their parties and admins

`team-engagement` SHALL allow `GetDispute` only to the dispute's claimant, its defendant, or a principal with `admin`; any
other signed-in caller SHALL receive `NOT_FOUND`, indistinguishable from an unknown dispute id (no dispute data in the
error, so dispute ids cannot be probed). Anonymous callers SHALL receive `UNAUTHENTICATED`.

#### Scenario: Stranger cannot read a dispute

- **WHEN** a buyer who is neither claimant nor defendant calls `GetDispute` for an existing dispute
- **THEN** the call is `NOT_FOUND` and no dispute data is returned

#### Scenario: Claimant and defendant can read

- **WHEN** the claimant buyer, then the defendant seller, call `GetDispute`
- **THEN** both receive the dispute

#### Scenario: Admin can read

- **WHEN** an admin calls `GetDispute`
- **THEN** the dispute is returned

### Requirement: Disputes are created only by the order's buyer against the order's seller

`team-engagement` SHALL verify, by reading the order from `team-order` through its service principal, on `CreateDispute`
that the order exists, that the caller is the order's buyer, and that `defendant_id` equals the order's seller id. An
unknown order, and an order the caller is not the buyer of, SHALL both be `NOT_FOUND` (indistinguishable, so order ids
cannot be probed); a `defendant_id` that is not the order's seller SHALL be `PERMISSION_DENIED`; an unreachable `team-order` or an unconfigured order client SHALL be `UNAVAILABLE`. In every rejected case no dispute SHALL
be stored.

#### Scenario: Buyer disputes their own order against its seller

- **WHEN** the order's buyer calls `CreateDispute` with that order id and `defendant_id` = the order's seller id
- **THEN** an `OPEN` dispute is created with the buyer as claimant

#### Scenario: Unknown order

- **WHEN** a buyer calls `CreateDispute` with an order id that does not exist
- **THEN** the call is `NOT_FOUND` and no dispute is stored

#### Scenario: Someone else's order

- **WHEN** buyer B calls `CreateDispute` for buyer A's order
- **THEN** the call is `NOT_FOUND` and no dispute is stored

#### Scenario: Wrong defendant

- **WHEN** the order's buyer calls `CreateDispute` with a `defendant_id` that is not the order's seller
- **THEN** the call is `PERMISSION_DENIED` and no dispute is stored

#### Scenario: Order service unavailable

- **WHEN** the order's buyer calls `CreateDispute` while `team-order` is unreachable
- **THEN** the call is `UNAVAILABLE` and no dispute is stored

### Requirement: The shop flag on an answer is derived from listing ownership

`team-engagement` SHALL set `is_shop_reply` on a stored answer to true only when the caller is the seller of the question's
listing (determined by reading the listing from `team-domain`), and SHALL ignore the `is_shop_reply` value in the request.
Any other signed-in caller with `engagement:write` SHALL still be able to answer, and their answer SHALL be stored with
`is_shop_reply=false`. When ownership cannot be determined the answer SHALL be stored with `is_shop_reply=false`.

#### Scenario: Seller's answer is a shop reply

- **WHEN** the listing's seller calls `AnswerQuestion` with `is_shop_reply=false`
- **THEN** the stored answer has `is_shop_reply=true`

#### Scenario: A buyer cannot claim to speak for the shop

- **WHEN** a buyer calls `AnswerQuestion` with `is_shop_reply=true`
- **THEN** the answer is stored with `is_shop_reply=false`

#### Scenario: Another seller cannot claim to speak for the shop

- **WHEN** a seller who does not own the listing calls `AnswerQuestion` with `is_shop_reply=true`
- **THEN** the answer is stored with `is_shop_reply=false`

#### Scenario: Ownership lookup fails

- **WHEN** the listing's seller answers while `team-domain` is unreachable
- **THEN** the answer is stored with `is_shop_reply=false`
