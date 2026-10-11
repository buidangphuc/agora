## 1. Code track

- [x] 1.1 team-ai: collection resolved from the serving pointer, pinned per request, alias fallback when no pointer (D1, D2); verify unit tests fail without the change (collection name from pointer, alias fallback, pin across memo expiry, missing generation collection fails open) and `make check` + `make test`
- [x] 1.2 platform-recsys: pointer-safe retention (empty keep deletes nothing; serving/previous spared without an alias), alias shim documented, rollback converge unchanged (D3-D5); verify `make test-host`
- [x] 1.3 Compose + env: team-ai `POSTGRES_*` via `TEAM_AI_POSTGRES_*` defaults and root `.env.example` (D6); verify `docker compose config` renders and no `listing_*` literal remains in the team-ai block

## 2. E2E track

- [x] 2.1 FEATURES.yaml acceptance line per scenario (status `planned`, note `needs rebuild`); verify `features-check`
- [x] 2.2 `recommendations/serving_switch_atomicity.feature` + `ssa_steps.py`, serving-pointer scenarios `@destructive`; verify lint and (integrator) run after rebuild

## 3. Gate

- [x] 3.1 `openspec validate serving-switch-atomicity --strict`, `spec_sync`, `repo_doctor`

## Evidence (2026-10-09)

- Gate after rebuilding identity, order, gateway, team-ai, analytics and frontend (images built from feat/ui-system):
  - parallel lane (`-n 4 -m "not destructive"`, deselecting test_ui_components): 710 passed and 4 xfailed, run twice
    (w2-par-1, w2-par-2). The 4 xfails are UI defects found by other tracks.
  - destructive lane: 75 passed, 3 failed (w2-destr). All 3 failures are in other changes' new tests
    (notification-delivery-hardening and ui-phase-cart-checkout) and are being fixed separately.
- Also `openspec validate --strict`, `spec_sync --strict` (e2e-ready), `features.py --strict` and repo_doctor.
- Review (contract-boundary) found no blocking issue. Its risks are recorded in design.md: overlapping publishes, the
  boot-time check, and a Redis blip during the 5 s memo.
- The six destructive scenarios passed in the serial lane.
