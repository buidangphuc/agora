## ADDED Requirements

### Requirement: A buyer escalates an escalatable return to a dispute

`EngagementService.CreateDispute` SHALL accept an optional `return_id`. When it is set, after the scope, validation and order-party checks pass, the service SHALL read the return from team-order with `GetReturnRequest` as the service principal with scope `order.read`, SHALL require the return to belong to the request's `order_id` and to have `can_escalate` true, and SHALL store the dispute `OPEN` with `return_id`, claimant the buyer and defendant the order's seller. With `return_id` set, a `defendant_id` other than the order's seller SHALL be `INVALID_ARGUMENT` and SHALL NOT call `GetReturnRequest`; without `return_id` the existing `PERMISSION_DENIED` for that case is unchanged. At most one dispute per return SHALL be `OPEN` or `INVESTIGATING` at a time. When `return_id` is empty the RPC SHALL behave as before and SHALL NOT call `GetReturnRequest`. The read of the return SHALL fail closed.

#### Scenario: Dispute escalation: Buyer escalates a rejected return
- **WHEN** the buyer of order "o-1" calls CreateDispute with order_id "o-1", the order's seller as defendant, a reason and return_id "r-1", and team-order reports "r-1" REJECTED with can_escalate true for order "o-1"
- **THEN** the call returns OK with a dispute in status OPEN, return_id "r-1", claimant the buyer and defendant the seller, and team-order received exactly 1 GetReturnRequest call whose principal is `service-team-engagement` with scope `order.read`
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeEscalatesRejectedReturn`

#### Scenario: Dispute escalation: Buyer escalates a return the seller never answered
- **WHEN** the buyer calls CreateDispute with the return_id of a PENDING return for which team-order reports can_escalate true
- **THEN** the call returns OK with a dispute in status OPEN linked to that return
- **VERIFIED BY**: platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Buyer escalates a return the seller never answered" (q10 compose overlay shortens the seller answer time)

#### Scenario: Dispute escalation: Return that cannot be escalated is refused
- **WHEN** the buyer calls CreateDispute with the return_id of a return for which team-order reports can_escalate false
- **THEN** the call returns `FAILED_PRECONDITION` and no dispute row exists for that return
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeReturnNotEscalatable`; platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Return that cannot be escalated is refused"

#### Scenario: Dispute escalation: Second dispute on the same return is refused while one is open
- **WHEN** the buyer calls CreateDispute for return "r-1" while an OPEN or INVESTIGATING dispute for "r-1" exists
- **THEN** the call returns `ALREADY_EXISTS` and the number of disputes for "r-1" stays 1
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeDuplicateOpenOnReturn`; platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Second dispute on the same return is refused while one is open"

#### Scenario: Dispute escalation: Concurrent creates leave one dispute
- **WHEN** 5 CreateDispute calls for the same return_id run concurrently against the database
- **THEN** exactly 1 call returns OK, the other 4 return `ALREADY_EXISTS`, and 1 dispute row exists for the return
- **VERIFIED BY**: team-engagement `internal/repository` `TestCreateDisputeUniqueOpenPerReturnConcurrent`

#### Scenario: Dispute escalation: A closed dispute lets the return be escalated again
- **WHEN** the only dispute for return "r-1" is REJECTED and the buyer calls CreateDispute for "r-1" while team-order still reports can_escalate true
- **THEN** the call returns OK with a new OPEN dispute and "r-1" has 2 dispute rows
- **VERIFIED BY**: team-engagement `internal/repository` `TestCreateDisputeAfterClosedDisputeOnReturn`

#### Scenario: Dispute escalation: Unknown return or return of another order
- **WHEN** the buyer calls CreateDispute with a return_id team-order does not know, or one whose order_id differs from the request's order_id
- **THEN** each call returns `NOT_FOUND` and no dispute is stored
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeReturnNotFoundOrWrongOrder`

#### Scenario: Dispute escalation: Not the buyer of the order
- **WHEN** a user who is not the buyer of order "o-1" calls CreateDispute with order_id "o-1" and a return_id
- **THEN** the call returns `NOT_FOUND` and team-order receives 0 GetReturnRequest calls
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeReturnForeignOrderNoReturnRead`

#### Scenario: Dispute escalation: Escalation naming another defendant is invalid
- **WHEN** the buyer of order "o-1" calls CreateDispute with return_id "r-1" and a defendant_id that is not the seller of "o-1"
- **THEN** the call returns `INVALID_ARGUMENT`, no dispute is stored and team-order receives 0 GetReturnRequest calls
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeReturnWrongDefendantInvalid`

#### Scenario: Dispute escalation: Team-order unavailable fails closed
- **WHEN** the buyer calls CreateDispute with a return_id while team-order is down, times out, or `UPSTREAM_ORDER_ADDR` is unset
- **THEN** the call returns `UNAVAILABLE` and no dispute is stored
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeReturnUpstreamUnavailable`; team-engagement `internal/upstream` `TestGetReturnTimeout`

#### Scenario: Dispute escalation: Dispute without return_id is unchanged
- **WHEN** the buyer calls CreateDispute with an empty return_id for their own order
- **THEN** the call returns OK with a dispute whose return_id is empty and team-order receives 0 GetReturnRequest calls
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeWithoutReturnUnchanged`

#### Scenario: Dispute escalation: Anonymous caller cannot create a dispute
- **WHEN** an anonymous caller calls CreateDispute with a return_id
- **THEN** the call returns `UNAUTHENTICATED` and team-order receives 0 calls
- **VERIFIED BY**: team-engagement `internal/handler` `TestCreateDisputeReturnAnonymousDenied`

### Requirement: Disputes persist their return link and ruling

The `disputes` table SHALL carry `return_id` and `ruling`, existing rows SHALL read back with an empty `return_id` and an unspecified ruling, and `GetDispute`, `ListOrderDisputes`, `ListDisputes` and `ResolveDispute` SHALL return both fields. The migration SHALL be reversible.

#### Scenario: Dispute escalation: Existing disputes survive the migration
- **WHEN** the migration runs on a database that holds a dispute created before it, then GetDispute reads that dispute
- **THEN** the dispute keeps its id, status and reason, with return_id "" and ruling unspecified, and the down migration removes both columns without touching other columns
- **VERIFIED BY**: team-engagement `internal/repository` `TestDisputeMigrationKeepsExistingRows`

#### Scenario: Dispute escalation: GetDispute shows the link and the ruling
- **WHEN** a party of a return-linked dispute that was RESOLVED with BUYER_FAVOUR calls GetDispute
- **THEN** the response carries return_id and ruling `DISPUTE_RULING_BUYER_FAVOUR`
- **VERIFIED BY**: team-engagement `internal/handler` `TestGetDisputeCarriesReturnAndRuling`

### Requirement: Dispute state machine is enforced

`EngagementService.ResolveDispute` SHALL allow only `OPEN -> INVESTIGATING`, `OPEN -> REJECTED` (spam), `INVESTIGATING -> INVESTIGATING` (note update), `INVESTIGATING -> RESOLVED` and `INVESTIGATING -> REJECTED`. Every other transition, including any move out of RESOLVED or REJECTED, SHALL return `FAILED_PRECONDITION` and write no audit event and no outbox row. RESOLVED on a return-linked dispute SHALL require a `ruling` (`INVALID_ARGUMENT` otherwise); a ruling sent with a status other than RESOLVED SHALL be `INVALID_ARGUMENT`. The status change SHALL be a compare-and-set on the previous status.

#### Scenario: Dispute escalation: Open dispute moves to investigating
- **WHEN** an admin calls ResolveDispute with status INVESTIGATING and a note on an OPEN dispute
- **THEN** the call returns OK, the status is INVESTIGATING, the note is stored, and no DisputeResolved row is written
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeOpenToInvestigating`

#### Scenario: Dispute escalation: Investigating note can be updated
- **WHEN** an admin calls ResolveDispute with status INVESTIGATING and a new note on an INVESTIGATING dispute
- **THEN** the call returns OK, the status stays INVESTIGATING and the stored note equals the new note
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeInvestigatingNoteUpdate`

#### Scenario: Dispute escalation: Open dispute cannot be resolved directly
- **WHEN** an admin calls ResolveDispute with status RESOLVED, with and without a ruling, on an OPEN dispute
- **THEN** each call returns `FAILED_PRECONDITION`, the status stays OPEN, 0 audit events are written and 0 DisputeResolved rows exist
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeOpenToResolvedRefused`; platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Open dispute cannot be resolved directly"

#### Scenario: Dispute escalation: Open dispute can be rejected as spam
- **WHEN** an admin calls ResolveDispute with status REJECTED and no ruling on an OPEN dispute
- **THEN** the call returns OK, the status is REJECTED, exactly 1 `dispute.resolve` audit event and exactly 1 DisputeResolved row with status REJECTED and ruling unspecified are written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeOpenToRejectedAllowed`

#### Scenario: Dispute escalation: Closed dispute stays closed
- **WHEN** an admin calls ResolveDispute with any status on a RESOLVED or REJECTED dispute
- **THEN** the call returns `FAILED_PRECONDITION`, the dispute is unchanged and 0 audit events are written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeClosedRefused`

#### Scenario: Dispute escalation: Return-linked resolution needs a ruling
- **WHEN** an admin calls ResolveDispute with status RESOLVED and ruling unspecified on an INVESTIGATING dispute that has a return_id
- **THEN** the call returns `INVALID_ARGUMENT`, the status stays INVESTIGATING and 0 audit events are written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveReturnLinkedWithoutRulingInvalid`

#### Scenario: Dispute escalation: Ruling on a non-resolving status is invalid
- **WHEN** an admin calls ResolveDispute with ruling BUYER_FAVOUR and status INVESTIGATING on an OPEN dispute, then with status REJECTED on an INVESTIGATING dispute
- **THEN** each call returns `INVALID_ARGUMENT`, the dispute is unchanged and 0 audit events are written
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveRulingWithNonResolvedStatusInvalid`

#### Scenario: Dispute escalation: Dispute without a return resolves without a ruling
- **WHEN** an admin calls ResolveDispute with status RESOLVED and no ruling on an INVESTIGATING dispute whose return_id is empty
- **THEN** the call returns OK and the status is RESOLVED
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeWithoutReturnNoRulingNeeded`

#### Scenario: Dispute escalation: Concurrent resolutions apply once
- **WHEN** two admins call ResolveDispute with RESOLVED at the same time on one INVESTIGATING dispute
- **THEN** exactly 1 call returns OK, the other returns `FAILED_PRECONDITION`, and exactly 1 DisputeResolved row is written
- **VERIFIED BY**: team-engagement `internal/repository` `TestResolveDisputeCompareAndSetOnce`

#### Scenario: Dispute escalation: Non-admin cannot rule
- **WHEN** the claimant, the defendant and an anonymous caller each call ResolveDispute with a ruling
- **THEN** the claimant and the defendant get `PERMISSION_DENIED`, the anonymous caller gets `UNAUTHENTICATED`, and the dispute is unchanged
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeRulingNonAdminDenied`

### Requirement: A ruling is audited first and announced once

`ResolveDispute` SHALL write the team-audit event `dispute.resolve` before it changes the dispute, with the existing metadata plus `ruling` (`buyer_favour`, `seller_favour` or empty) and `return_id` (empty when none), and SHALL fail closed with `UNAVAILABLE`. When the dispute reaches RESOLVED or REJECTED, the same database transaction that updates the status SHALL write exactly one `DisputeResolved` fact (`dispute_id`, `order_id`, `return_id`, `status`, `ruling`, `resolver_id` = the admin's user id from the verified Principal, `resolved_at`) to the outbox, relayed to `engagement.events` with key = dispute id. No `DisputeResolved` SHALL be written for INVESTIGATING.

#### Scenario: Dispute escalation: Buyer-favour ruling writes the audit event and one fact
- **WHEN** admin "adm-1" resolves INVESTIGATING dispute "d-1" (order "o-1", return "r-1") with status RESOLVED and ruling BUYER_FAVOUR
- **THEN** exactly one `dispute.resolve` audit event is written before the update with metadata ruling "buyer_favour" and return_id "r-1", and exactly one DisputeResolved row exists with status RESOLVED, ruling BUYER_FAVOUR, resolver_id "adm-1"
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeBuyerFavourAuditAndEvent`

#### Scenario: Dispute escalation: Rejected dispute carries no ruling
- **WHEN** an admin moves an INVESTIGATING dispute to REJECTED with no ruling
- **THEN** one DisputeResolved row is written with status REJECTED and ruling unspecified
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeRejectedEventNoRuling`

#### Scenario: Dispute escalation: Seller-favour ruling is announced
- **WHEN** an admin resolves an INVESTIGATING return-linked dispute with ruling SELLER_FAVOUR
- **THEN** one DisputeResolved row is written with ruling SELLER_FAVOUR
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeSellerFavourEvent`

#### Scenario: Dispute escalation: Audit is written before the update
- **WHEN** ResolveDispute runs for an INVESTIGATING dispute
- **THEN** the recorded call order is the audit write, then the status update with its outbox row
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeAuditBeforeUpdateAndEvent`

#### Scenario: Dispute escalation: Audit down blocks the ruling
- **WHEN** team-audit is down or `UPSTREAM_AUDIT_ADDR` is unset during ResolveDispute on an INVESTIGATING dispute
- **THEN** the call returns `UNAVAILABLE`, the status stays INVESTIGATING and 0 DisputeResolved rows are written
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeAuditDownNoEvent`; platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Audit down blocks the ruling"

#### Scenario: Dispute escalation: A failed update leaves no event
- **WHEN** the audit write succeeds and the status update fails
- **THEN** the call returns `INTERNAL` and 0 DisputeResolved rows exist; a retry on a healthy store returns OK and writes exactly 1 row
- **VERIFIED BY**: team-engagement `internal/repository` `TestResolveDisputeUpdateFailureWritesNoFact`

#### Scenario: Dispute escalation: A refused ruling writes no audit event and no fact
- **WHEN** a non-admin caller is denied by ResolveDispute with a ruling
- **THEN** the response is `PERMISSION_DENIED`, 0 audit events are written and 0 DisputeResolved rows exist
- **VERIFIED BY**: team-engagement `internal/handler` `TestResolveDisputeDeniedWritesNoAuditNoFact`

#### Scenario: Dispute escalation: The fact reaches Kafka once
- **WHEN** an admin resolves a return-linked dispute through the gateway with ruling BUYER_FAVOUR
- **THEN** within 10 seconds `engagement.events` holds exactly 1 envelope of type `platform.engagement.v1.DisputeResolved` keyed by the dispute id whose payload has that return_id and ruling BUYER_FAVOUR
- **VERIFIED BY**: platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: The fact reaches Kafka once" (uses `prm_rma_support.scan`)

### Requirement: Parties and admins list the disputes of an order

`EngagementService.ListOrderDisputes` SHALL require scope `engagement:read`, SHALL return the disputes of `order_id` newest first to the order's buyer, the order's seller or an admin, and SHALL return `PERMISSION_DENIED` to any other caller, also when the order does not exist. A non-admin check SHALL use the order's parties from team-order and SHALL fail closed.

#### Scenario: Dispute escalation: Buyer and seller list the order's disputes newest first
- **WHEN** the buyer, then the seller, of an order with 2 disputes call ListOrderDisputes
- **THEN** each response holds the 2 disputes with the newer first
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesPartiesNewestFirst`; platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Buyer and seller list the order's disputes newest first"

#### Scenario: Dispute escalation: Admin lists any order's disputes
- **WHEN** an admin calls ListOrderDisputes for an order they are not a party of while team-order is down
- **THEN** the response holds the order's disputes and team-order receives 0 calls
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesAdminNoUpstream`

#### Scenario: Dispute escalation: Non-party is denied
- **WHEN** a signed-in user who is neither buyer nor seller of the order calls ListOrderDisputes
- **THEN** the call returns `PERMISSION_DENIED` and no dispute is returned
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesNonPartyDenied`; platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Non-party is denied"

#### Scenario: Dispute escalation: Unknown order gives the same answer as a foreign order
- **WHEN** a non-admin calls ListOrderDisputes with an order id team-order does not know
- **THEN** the call returns `PERMISSION_DENIED`
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesUnknownOrderDenied`

#### Scenario: Dispute escalation: Order without disputes lists empty
- **WHEN** the buyer calls ListOrderDisputes for their order that has no dispute
- **THEN** the response holds 0 disputes and the status is OK
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesEmpty`

#### Scenario: Dispute escalation: List needs an order id and a login
- **WHEN** a signed-in user calls ListOrderDisputes with an empty order_id, and an anonymous caller calls it with an order id
- **THEN** the first call returns `INVALID_ARGUMENT` and the second returns `UNAUTHENTICATED`
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesValidationAndAnonymous`

#### Scenario: Dispute escalation: List without engagement read scope is denied
- **WHEN** a signed-in buyer of the order whose token lacks scope `engagement:read` calls ListOrderDisputes
- **THEN** the call returns `PERMISSION_DENIED`, no dispute is returned and team-order receives 0 calls
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesMissingScopeDenied`

#### Scenario: Dispute escalation: Team-order down blocks a non-admin list
- **WHEN** the buyer calls ListOrderDisputes while team-order is down
- **THEN** the call returns `UNAVAILABLE`
- **VERIFIED BY**: team-engagement `internal/handler` `TestListOrderDisputesUpstreamUnavailable`

### Requirement: Admins list disputes by status

`EngagementService.ListDisputes` SHALL require the `admin` scope, SHALL filter by `status` (unspecified = every status), SHALL return disputes oldest first by creation time with a cursor page, SHALL default the page size to 20 and clamp it to 100, and SHALL write no audit event.

#### Scenario: Dispute escalation: Admin lists open disputes oldest first
- **WHEN** an admin calls ListDisputes with status OPEN while 3 OPEN and 2 RESOLVED disputes exist
- **THEN** the response holds the 3 OPEN disputes ordered by created_at ascending
- **VERIFIED BY**: team-engagement `internal/repository` `TestListDisputesByStatusOldestFirst`; platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Admin lists open disputes oldest first"

#### Scenario: Dispute escalation: Pagination boundary
- **WHEN** an admin calls ListDisputes with page_size 2 over 3 disputes, then with the returned cursor
- **THEN** the first page holds 2 disputes with a next cursor and the second holds 1 dispute with no next cursor; page_size 500 returns at most 100
- **VERIFIED BY**: team-engagement `internal/repository` `TestListDisputesPagination`

#### Scenario: Dispute escalation: Listing writes no audit event
- **WHEN** an admin calls ListDisputes
- **THEN** 0 audit events are written
- **VERIFIED BY**: team-engagement `internal/handler` `TestListDisputesNotAudited`

#### Scenario: Dispute escalation: Non-admin and anonymous are refused
- **WHEN** a buyer, a seller and an anonymous caller each call ListDisputes
- **THEN** the buyer and the seller get `PERMISSION_DENIED` and the anonymous caller gets `UNAUTHENTICATED`
- **VERIFIED BY**: team-engagement `internal/handler` `TestListDisputesNonAdminDenied`

### Requirement: Dispute escalation works end to end

With the q10 compose overlay (short seller answer time) and the e2e stack running team-order, team-engagement, team-audit and Kafka, a buyer SHALL be able to escalate a rejected return and an admin SHALL be able to work the dispute to a ruling through the gateway.

#### Scenario: Dispute escalation: Rejected return to resolved dispute end to end
- **WHEN** a buyer's return is rejected by the seller, the buyer escalates it, and an admin moves the dispute to INVESTIGATING and then to RESOLVED with ruling BUYER_FAVOUR through the gateway
- **THEN** GetDispute shows RESOLVED with ruling BUYER_FAVOUR and the return_id, and QueryAuditLog for the dispute returns 2 `dispute.resolve` events, the last with metadata ruling "buyer_favour"
- **VERIFIED BY**: platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Rejected return to resolved dispute end to end"

#### Scenario: Dispute escalation: Return keeps its status until the refund consumer acts
- **WHEN** a dispute is moved to INVESTIGATING
- **THEN** the linked return's status in team-order stays unchanged
- **VERIFIED BY**: platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: Return keeps its status until the refund consumer acts"

#### Scenario: Dispute escalation: A buyer cannot rule on their own dispute end to end
- **WHEN** the buyer who opened a return-linked dispute calls ResolveDispute with ruling BUYER_FAVOUR through the gateway without the admin scope
- **THEN** the call is denied with `PERMISSION_DENIED`, the dispute stays unchanged and QueryAuditLog for the dispute returns 0 events
- **VERIFIED BY**: platform-e2e `engagement/dispute_escalation.feature` "Dispute escalation: A buyer cannot rule on their own dispute end to end"
