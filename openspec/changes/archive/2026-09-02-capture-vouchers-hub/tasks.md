# Tasks

## 1. Code (per repo)
- [x] None — the Vouchers Hub already exists in team-frontend and seed data.

## 2. E2E (platform-e2e)
- [ ] Add `promo.vouchers-page` to `team-frontend/FEATURES.yaml` (status: automated).
- [ ] Add `tests/e2e/features/promo/vouchers.feature` + `promo_steps.py` + binder.
- [ ] Run against the live stack; confirm green.
- [ ] `make -C platform-e2e features-check` green.

## 3. Archive
- [ ] `make -C platform-e2e spec-check CHANGE=capture-vouchers-hub` green.
- [ ] `openspec archive capture-vouchers-hub`.
