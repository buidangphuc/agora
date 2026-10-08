## Purpose

Defines who may submit, read and review KYC records in `team-verification`, so a user cannot approve their own verification and no unauthenticated caller shares an identity with another.

## ADDED Requirements

### Requirement: KYC submit and own-status require an authenticated caller

`team-verification` SHALL require an authenticated principal (not absent, not the anonymous principal) on `SubmitKyc` and
on `GetVerificationStatus`, and SHALL derive the caller's identity only from that principal. It SHALL NOT substitute a
fallback or demo identity when no principal is present. A call without a principal or with the anonymous principal SHALL
receive `UNAUTHENTICATED` and no submission SHALL be created.

#### Scenario: Anonymous caller cannot submit KYC

- **WHEN** an anonymous caller calls `SubmitKyc`
- **THEN** the call is `UNAUTHENTICATED` and no submission is stored

#### Scenario: A call without any principal has no identity

- **WHEN** a call reaches `team-verification` with no `x-principal-id` metadata and calls `SubmitKyc`
- **THEN** the call is `UNAUTHENTICATED` and no submission is stored for any demo user

#### Scenario: A user submits and sees PENDING

- **WHEN** a logged-in buyer calls `SubmitKyc` and then `GetVerificationStatus` with their own user id (or an empty id)
- **THEN** the status is `PENDING`

### Requirement: Only an admin reviews KYC

`team-verification` SHALL require the scope `admin` on `ReviewKyc`. A signed-in caller without `admin` SHALL receive
`PERMISSION_DENIED` and the submission SHALL be unchanged; an anonymous caller SHALL receive `UNAUTHENTICATED`.

#### Scenario: A user cannot approve their own KYC

- **WHEN** a buyer submits KYC and then calls `ReviewKyc` with `APPROVE` for their submission id
- **THEN** the call is `PERMISSION_DENIED` and the status is still `PENDING`

#### Scenario: Anonymous caller cannot review

- **WHEN** an anonymous caller calls `ReviewKyc`
- **THEN** the call is `UNAUTHENTICATED`

#### Scenario: Seeded admin approves a submission

- **WHEN** the seeded admin calls `ReviewKyc` with `APPROVE` for a pending submission
- **THEN** the user's status becomes `VERIFIED` and the badge is true

### Requirement: Reading another user's verification status is admin-only

`team-verification` SHALL honour a `user_id` in `GetVerificationStatus` only when it equals the caller's own id or the
caller holds `admin`; any other signed-in caller naming a different user SHALL receive `PERMISSION_DENIED` with no status
data.

#### Scenario: A buyer cannot read another user's status

- **WHEN** a buyer calls `GetVerificationStatus` with another user's id
- **THEN** the call is `PERMISSION_DENIED`

#### Scenario: An admin can read any user's status

- **WHEN** an admin calls `GetVerificationStatus` with a user's id
- **THEN** that user's status and badge are returned
