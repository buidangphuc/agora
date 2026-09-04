## Why

The marketplace already ships a Vouchers Hub (`/vouchers`, seeded by
`platform-core/tools/seed/02-vouchers.sh`) but it has no spec and no e2e coverage.
This change captures the capability as a spec and adds e2e so it is guarded and
appears in the coverage map. It is the pilot for the OpenSpec → spec-to-e2e pipeline.

## What Changes

- Add a `promo-vouchers` capability spec describing the Vouchers Hub browse behavior.
- Add the `promo.vouchers-page` feature to `team-frontend/FEATURES.yaml`.
- Add a platform-e2e scenario that verifies a visitor can browse available vouchers.

## Non-goals

- No frontend/backend code change (behavior already exists).
- Saving/redeeming a voucher end-to-end (separate change).
- A dedicated voucher service (vouchers are seeded data + frontend for now).
