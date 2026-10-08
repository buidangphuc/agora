## Purpose

Defines that `team-referral` treats the gateway's anonymous principal as unauthenticated, so logged-out callers cannot create,
read or redeem referrals as one shared user.

## ADDED Requirements

### Requirement: The anonymous principal is not a referral user

`team-referral` SHALL treat a caller as authenticated only when the forwarded principal id is non-empty and not the literal
`anonymous`, and the forwarded principal type is not `anonymous`. `CreateReferralCode`, `GetMyReferral`, `RedeemReferral` and
`ListReferralRewards` SHALL return `UNAUTHENTICATED` for any other caller, with nothing created, redeemed or listed.

#### Scenario: A logged-out caller cannot create or redeem a referral code

- **WHEN** an anonymous caller (gateway principal `anonymous`, type `anonymous`) calls `CreateReferralCode` and `RedeemReferral`
- **THEN** both calls are `UNAUTHENTICATED` and no code or redemption row exists

#### Scenario: A logged-out caller cannot list rewards

- **WHEN** an anonymous caller calls `GetMyReferral` and `ListReferralRewards`
- **THEN** both calls are `UNAUTHENTICATED`

#### Scenario: A signed-in buyer keeps the referral flow

- **WHEN** a signed-in buyer creates a code, reads it back with `GetMyReferral` and another buyer redeems it
- **THEN** the code is returned unchanged and the redemption is recorded for the two real user ids
