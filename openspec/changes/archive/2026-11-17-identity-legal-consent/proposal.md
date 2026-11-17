## Why

Today a visitor registers without accepting any Terms or Privacy Policy (`RegisterRequest` has no version field,
`identity.proto:111-123`), and consent choices live only in the `agora_consent` cookie. Only seller terms are
recorded (`users.seller_terms_version`, migration `0021_seller_terms`). Backlog Q24 needs server-side proof of consent
(Decree 13/2023 Art. 9-11 as cited by the initiative, unverified, see NEEDS_HUMAN) before account deletion and export can
be built on top.

initiative: trust-legal

## What changes

- `AuthService.Register` requires `terms_version` and `privacy_version`; both must equal the current configured
  version (`LEGAL_TERMS_VERSIONS`, `PRIVACY_POLICY_VERSIONS`, first entry is current). The account and its two consent
  rows (source `register`) are written in one transaction; a missing or stale version fails `INVALID_ARGUMENT` and no
  account exists.
- New append-only consent ledger table in team-identity (migration `0022_consents`).
- New `PrivacyService` gRPC service on the existing identity server. This slice implements `RecordConsent` and
  `ListMyConsents`. `RequestAccountDeletion`, `CancelAccountDeletion`, `GetAccountDeletionStatus` (slice
  identity-account-deletion) and `ExportMyData` (slice identity-data-export) answer `UNIMPLEMENTED` until those slices land.
- team-gateway forwards `PrivacyService` (Connect) for signed-in users in `internal/edge/auth.go`; the four other RPCs stay
  unimplemented at the edge.
- Config `LEGAL_TERMS_VERSIONS` and `PRIVACY_POLICY_VERSIONS`, validated at boot like `SELLER_TERMS_VERSIONS`.
- Re-acceptance: a user whose latest accepted TERMS or PRIVACY_POLICY version differs from the current one is
  detectable from `ListMyConsents`; the frontend slice (frontend-legal-pages) shows the prompt and calls `RecordConsent`.
  Legacy users have no rows, so they are prompted on next sign-in.

Contract consumed (from slice trust-legal-contract, not redefined here): `RegisterRequest.terms_version=5`,
`privacy_version=6`, `PrivacyService`, `ConsentPurpose`, `ConsentRecord`, `RecordConsent*`, `ListMyConsents*`.

## Capabilities

### New Capabilities
- `identity-consent`: legal-version acceptance at sign-up and the per-user consent ledger.

### Modified Capabilities
none

## Impact

- team-identity: `internal/config/config.go`, `internal/identity/auth.go` (Register), `internal/store` (migration, repo),
  new `internal/rpc/privacy.go`, `internal/authz/scopes.go` (user-principal rule for PrivacyService), `internal/app/app.go`
  registration, `FEATURES.yaml` (by e2e-author), docker-compose env.
- team-gateway: `internal/edge/auth.go` (PrivacyService forwarder; hot file shared with identity-account-deletion and
  identity-data-export, serialised by initiative-build).
- Existing callers of `Register` (seed scripts, e2e register helpers, frontends) must send both versions; the seed and
  platform-e2e helper updates are in the e2e track.

## Non-goals

Deletion, export, the legal page content and the banner UI (other slices). Consent for anonymous visitors (cookie only).
Backfilling consent rows for legacy users. Reading consent in other services.

## Non-functional requirements

- `RecordConsent` p95 <= 50 ms at 20 rps locally; `ListMyConsents` p95 <= 100 ms for 1000 records.
- Register adds no extra round trip: consent rows are inserted in the existing registration transaction.
- Consent writes are fail closed: a database error returns `INTERNAL`/`UNAVAILABLE` and never reports success.
- Consent table is append-only: no RPC updates or deletes a row.
- Page size default 20, clamp 100.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `consents.user_id` | personal (pseudonymous id) | link proof to account | legal obligation to prove consent | account life; kept after erasure for N years (N set by legal, NEEDS_HUMAN) |
| `consents.purpose`, `granted`, `version`, `source` | personal (preferences) | proof of what was accepted | consent evidence | same |
| `consents.recorded_at` | personal | proof of when | consent evidence | same |

No IP address or user agent is stored on consent rows. Consent payloads and user ids are never logged (go-sdk sloglint
forbidden keys); logs carry the record count and purpose names only.

## Threat model

- Assets: consent evidence integrity (who accepted which version when), account creation path.
- Abuse cases: (1) forging consent for another user: the user id always comes from the principal, never the request.
  (2) Back-dating: `recorded_at` is server-set and ignored on input. (3) Registering without acceptance by omitting or
  sending a stale version: rejected with `INVALID_ARGUMENT`. (4) Anonymous or service principals writing consent:
  `UNAUTHENTICATED` / `PERMISSION_DENIED`. (5) Ledger flooding: at most 6 records per call, 1 record per purpose, edge
  rate limits apply. (6) Rewriting history: no update/delete path exists.
- Authz rule: PrivacyService is USER principal only; a user can read and write only their own ledger.
