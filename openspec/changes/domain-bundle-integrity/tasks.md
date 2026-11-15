## 1. Code track (team-domain)

- [x] 1.1 Tests first in `team-domain/internal/listing/bundle_test.go`: table tests for B1-B3, B5-B11 against a fake listing repo; verify red (B2, B3, B5-B11 fail on current code) (B1-B3, B5-B11)
- [x] 1.2 `BundleService` takes the listing repository; `Create` validates count 2..10, no duplicates, existence and ownership (one answer for foreign and unknown), no variants, price in (0, sum) using one batched read by id; add sentinel errors (B1-B3, B5-B11)
- [x] 1.3 `team-domain/internal/rpc/errors.go`: map count/duplicate/variant/price errors to `INVALID_ARGUMENT`, foreign/missing to `PERMISSION_DENIED` with a single message (B2, B3, B5-B9, B11)
- [x] 1.4 `team-domain/internal/app/app.go` wiring of the new dependency; no flag, no fallback (B1)
- [x] 1.5 `team-domain/internal/rpc/bundle_test.go`: handler test for the scope check and for `GetBundle`/`ListBundlesBySeller` returning a legacy bad bundle (B4, B12, B13)
- [ ] 1.6 Run `gate_digest.py` for team-domain with `GO_SDK_REQUIRE_DOCKER=1`; lint (sloglint) clean (B1-B12)

## 2. e2e track (platform-e2e, team-domain/FEATURES.yaml)

- [x] 2.1 `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` with one scenario per id B1-B13, seeded through the real gateway API (listings by two sellers, one with variants); a legacy bundle for B12 seeded by SQL fixture into the team-domain database (B1-B12)
- [x] 2.2 Step definitions in `platform-e2e/tests/e2e/step_definitions/` reusing the seller login and listing seeding steps (B1-B12)
- [x] 2.3 `team-domain/FEATURES.yaml` entry `bundle.create` with `api.exercised: [listing.v1.CreateBundle, listing.v1.GetBundle, listing.v1.ListBundlesBySeller]` (B1, B12, B13)
- [x] 2.4 `openspec validate domain-bundle-integrity --strict` and `spec_lint.py` pass (B1-B12)
