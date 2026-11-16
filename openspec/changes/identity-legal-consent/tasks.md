## 1. Code track (team-identity, team-gateway)

Depends on slice trust-legal-contract (generated stubs for `terms_version`, `privacy_version`, `PrivacyService`).

- [ ] 1.1 Unit tests first (red): config validation for `LEGAL_TERMS_VERSIONS` / `PRIVACY_POLICY_VERSIONS` (scenarios: Boot fails without a configured version, First entry is the current version)
- [ ] 1.2 Config: add `LegalVersions` settings next to `SellerTerms` in `internal/config/config.go`; env in `docker-compose.local.yaml` and root `docker-compose.services.yaml`
- [ ] 1.3 Migration `0022_consents` (up/down): table `consents(id bigserial, user_id, purpose, granted, version, source, recorded_at)` with index `(user_id, recorded_at desc, id desc)`; no update/delete grants in repo code (scenario: ledger append-only)
- [ ] 1.4 Store: `AppendConsents` (atomic, one tx) and `ListConsents` (keyset cursor); integration tests with Docker (`GO_SDK_REQUIRE_DOCKER=1`) for paging 10/10/5, isolation between users, rollback (scenarios: History is newest first and paged, A user never sees another user's records, Consent insert failure rolls back the account)
- [ ] 1.5 `Register`: validate versions, insert two rows inside `CreateRegistered` transaction (scenarios: Register with the current versions stores two consent records, Register without versions is rejected, Register with a stale version is rejected)
- [ ] 1.6 `internal/rpc/privacy.go`: `PrivacyHandler` with `RecordConsent`, `ListMyConsents`; other four RPCs `UNIMPLEMENTED`; validation rules; user id from principal; no raw ids/payloads in logs (scenarios: Banner choice stores one row per changed purpose, Re-accepting a new Terms version appends a row, Invalid records are rejected without partial writes, Stale or withdrawn legal acceptance is rejected, Client time and user id are ignored, Page size is clamped and bad cursors are rejected, A user with no records gets an empty list)
- [ ] 1.7 Authz: register PrivacyService in `internal/app/app.go`; user-principal-only rule in `internal/authz/scopes.go`; extend `coverage_test.go` (scenarios: Anonymous and service principals cannot record consent, Anonymous caller cannot list)
- [ ] 1.8 team-gateway: `PrivacyForwarder` in `internal/edge/auth.go` for the two RPCs, others unimplemented; wire in the gateway app; unit test with a fake client (scenarios: Anonymous edge call is refused, Later-slice RPCs are not exposed yet)
- [ ] 1.9 Gates through `scripts/asdlc/gate_digest.py` for team-identity and team-gateway

## 2. E2E track (platform-e2e; FEATURES.yaml by e2e-author)

- [ ] 2.1 Feature file `identity/legal_consent.feature` covering every scenario in `specs/identity-consent/spec.md`; replace each `VERIFIED BY: TBD` with its path and scenario title
- [x] 2.2 Update every existing `Register` call (platform-e2e helpers, `platform-core/tools/seed-marketplace.sh`, compose seed) to send both current versions (scenario: Register with the current versions stores two consent records)
- [ ] 2.3 Step for a changed Terms version using a stack started with `LEGAL_TERMS_VERSIONS=<new>,<old>` (scenarios: Re-accepting a new Terms version appends a row, First entry is the current version)
- [ ] 2.4 `team-identity/FEATURES.yaml` entries: `identity.register_consent`, `identity.consent_ledger` (e2e-author)
