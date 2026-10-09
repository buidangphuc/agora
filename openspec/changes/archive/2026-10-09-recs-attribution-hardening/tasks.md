## 1. Code track (team-analytics)

- [x] 1.1 `RecommendationPerformance`: impression identity, click-in-impression matching and deterministic crediting (D2); verify DuckDB tests that fail on the old query (a click on another listing is not counted; a reused id under two placements gives two rows and the click goes to the matching one, and without placement to the latest event; a click before its impression is not counted)
- [x] 1.2 Purchases from `order_facts` (D1) and mature-click conversion rate (D3, `MatureClicks`/`MaturePurchases` on `PerformanceRow`, service computes the rate); verify tests: beacon purchases give 0, a PAID line of the buyer is credited once to the earliest click, other buyers/listings/NULL buyer/after-window lines are not, a click inside the attribution window of the report end is excluded from the rate but present in `clicks` and `purchases`, then `make check`

## 2. E2E track (platform-e2e; team-analytics FEATURES.yaml)

- [x] 2.1 One FEATURES.yaml acceptance line per new/renamed scenario, `planned` with note `needs rebuild`; verify `features-check`
- [x] 2.2 `analytics/recs_attribution_hardening.feature` (+ `rah_` steps): paid order attributed, forged beacon, foreign-listing click, reused id, open window; verify lint clean and collection
- [x] 2.3 Integrator: remove the roe scenario "A purchase after a recommended click is attributed" from `recommendation_performance.feature`, `roe_steps.py` and team-analytics FEATURES.yaml (it posts a purchase beacon, which no longer counts; the same-named scenario in `recs_attribution_hardening.feature` replaces it); verify `spec-check`

## 3. Review and verify

- [x] 3.1 Gate: `openspec validate recs-attribution-hardening --strict`, `spec_sync`, `repo_doctor`; team-analytics rebuilt (with `order-facts-buyer`), new scenarios green

## Evidence (2026-10-09)

- Gate after rebuilding identity, order, gateway, team-ai, analytics and frontend (images built from feat/ui-system):
  - parallel lane (`-n 4 -m "not destructive"`, deselecting test_ui_components): 710 passed and 4 xfailed, run twice
    (w2-par-1, w2-par-2). The 4 xfails are UI defects found by other tracks.
  - destructive lane: 75 passed, 3 failed (w2-destr). All 3 failures are in other changes' new tests
    (notification-delivery-hardening and ui-phase-cart-checkout) and are being fixed separately.
- Also `openspec validate --strict`, `spec_sync --strict` (e2e-ready), `features.py --strict` and repo_doctor.
- Integrator changes:
  - the default report window is now 168 h, so `conversion_rate` is not structurally 0;
  - the roe beacon-purchase scenario was removed;
  - the roe no-click scenario now pays a real order.
- Follow-up for platform-core (not this change): `open_window_clicks` on `RecommendationPerformanceRow`, and a fix to
  the `conversion_rate` proto comment (D4).
