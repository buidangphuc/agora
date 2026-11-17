## 1. Code track (team-domain)

- [x] 1.1 Add sentinel errors and constants (tagline 120, 6 featured, theme set, banner 2048) in `internal/listing/storefront.go`; map them to INVALID_ARGUMENT in `internal/rpc/errors.go`. Scenarios: tagline, theme, banner, featured requirements.
- [x] 1.2 Tagline and theme validation in `StorefrontService.Upsert`, trimmed tagline stored. Scenarios: Valid tagline and theme are saved; Tagline of exactly 120 runes; Tagline over 120; Tagline with a control character; Unknown theme; Empty theme.
- [x] 1.3 Banner validation against the storage public base (shared helper with `internal/objects/storage.go`), unchanged-legacy exemption via `GetBySeller`; inject public base in `NewStorefrontService` and the wiring in `cmd/`. Scenarios: External banner URL; Non-http scheme, query string or fragment; Banner can be cleared; A legacy banner already stored.
- [x] 1.4 Featured validation with one batched listing read (extend the listing repository port and its Postgres implementation; no N+1). Scenarios: Seventh featured listing; Duplicate or empty ids; Unknown and foreign same answer; Featured lookup uses one batched read; Lookup failure stores nothing.
- [x] 1.5 Keep handler owner/scope behaviour; add handler tests. Scenarios: Principal without listing.write is denied; Body seller_id of another seller is ignored; Existing display name and slug rules still apply.
- [ ] 1.6 Run `make` gates for team-domain through `gate_digest.py` with `GO_SDK_REQUIRE_DOCKER=1`; sloglint clean (no tagline/banner/ids in logs).

## 2. E2E track (platform-e2e)

- [x] 2.1 Add `tests/e2e/features/domain/storefront_customisation.feature` seeded through the real gateway (seller login, listings created via API, image upload via `GetImageUploadUrl`): scenarios Seller saves a banner uploaded through the image bucket; Seller saves six featured listings in order; Seller cannot feature another seller's listing; Unpublished featured listing does not break reading the storefront; Changing only the tagline keeps banner, featured listings and theme.
- [x] 2.2 Step definitions reuse existing seller/listing helpers; no fake containers or route fulfils.
- [x] 2.3 Handoff: e2e-author adds the FEATURES.yaml entry in `team-domain/FEATURES.yaml` (API-level, `listing.v1.UpsertStorefront`).
