# Design

## Clarifications

### Round 1 — 2026-10-16

- M4 [money] Seller hold: Stock cannot be reserved on a held listing: May ReserveStock or checkout proceed on a HELD listing? → **No; ReserveStock with a new reservation_id returns FAILED_PRECONDITION and stock is unchanged; an existing reservation id retry stays a no-op success** (human)
- A10 [trust] Seller hold: Owner cannot delete a held listing: May the seller delete a HELD listing? → **No; DeleteListing on HELD returns FAILED_PRECONDITION (BANNED is owned by domain-listing-moderation)** (human)
- A13 [privacy] Seller hold: Hold event carries the status for de-indexing: May ListingChanged carry the moderation note or screen flags? → **No; the event carries only status and the fixed reason code** (human)
- A15 [lifecycle] Seller hold: Suspension lifted at login restores listings: What happens to listings when a suspension expires and the seller never logs in? → **They stay HELD until the next login; a lift sweeper is an identity follow-up (stated in proposal Non-functional requirements)** (human)
- R1 [integrity] Seller hold: Re-driven older suspension does not re-hold a reinstated seller: How is a DLQ re-drive or late older event handled? → **Skip any UserStatusChanged whose changed_at is not newer than the last one applied for that user (table seller_status_applied), recording its event id** (human)
- R2 [limits] Seller hold: Suspension holds published and pending listings: What happens above 5,000 listings? → **One transaction in batches of 500 with no cap; 5,000 is only the tested bound** (human)
- R3 [errors] Seller hold: Malformed event is parked, not applied: How is an unknown or future UserStatus value handled? → **Treated as malformed and parked on the DLQ** (human)
- R4 [security] Seller hold: Other event types are ignored: How are forged UserStatusChanged events prevented? → **Kafka ACL on identity.events, accepted as a deploy item for LOCK.md's human list** (human)

### Round 2 — 2026-10-18
- Q1 [data] Proposal config/NFR: the proposal named group `team-domain-seller-hold` and DLQ `identity.events.team-domain.dlq`, the merged code uses `team-domain-identity` (`team-domain/internal/config/config.go:79`) and `identity.events.dlq` (`internal/consumer/consumer.go:112`). Which is right? → **Align the proposal to the code; no behaviour change** (human)
