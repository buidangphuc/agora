## 1. Code track (team-analytics)

- [ ] 1.1 `RecommendationPerformance`: impression identity, click-in-impression matching and deterministic crediting (D2); verify DuckDB tests that fail on the old query (a click on another listing is not counted; a reused id under two placements gives two rows and the click goes to the matching one, and without placement to the latest event; a click before its impression is not counted)
- [ ] 1.2 Purchases from `order_facts` (D1) and mature-click conversion rate (D3, `MatureClicks`/`MaturePurchases` on `PerformanceRow`, service computes the rate); verify tests: beacon purchases give 0, a PAID line of the buyer is credited once to the earliest click, other buyers/listings/NULL buyer/after-window lines are not, a click inside the attribution window of the report end is excluded from the rate but present in `clicks` and `purchases`, then `make check`

## 2. E2E track (platform-e2e; team-analytics FEATURES.yaml)

- [ ] 2.1 One FEATURES.yaml acceptance line per new/renamed scenario, `planned` with note `needs rebuild`; verify `features-check`
- [ ] 2.2 `analytics/recs_attribution_hardening.feature` (+ `rah_` steps): paid order attributed, forged beacon, foreign-listing click, reused id, open window; verify lint clean and collection
- [ ] 2.3 Integrator: remove the roe scenario "A purchase after a recommended click is attributed" from `recommendation_performance.feature`, `roe_steps.py` and team-analytics FEATURES.yaml (it posts a purchase beacon, which no longer counts; the same-named scenario in `recs_attribution_hardening.feature` replaces it); verify `spec-check`

## 3. Review and verify

- [ ] 3.1 Gate: `openspec validate recs-attribution-hardening --strict`, `spec_sync`, `repo_doctor`; team-analytics rebuilt (with `order-facts-buyer`), new scenarios green
- [ ] 3.2 Follow-up for platform-core (not this change): `open_window_clicks` on `RecommendationPerformanceRow` and the `conversion_rate` comment (D4)
