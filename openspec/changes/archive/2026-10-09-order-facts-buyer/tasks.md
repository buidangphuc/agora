## 1. Code track

- [x] 1.1 team-analytics: `buyer_id` in `OrderFactRecord`/`OrderFactsSchema` (D1), consumer maps it (empty -> NULL), DuckDB writer + in-place migration, BigQuery writer + `evolveSchema` for `order_facts` (D3); verify tests that fail without the change (consumer maps the buyer and empty -> NULL; DuckDB round trip and migration of a table without the column keeps old rows with NULL; Parquet export has the column; schema parity), then `make check`
- [x] 1.2 platform-featurestore: `user_activity@v2` with `paid_orders_30d` (D5), v1 retired, lock regenerated, `buyer_id` required in the orders input (D6), README; verify pytest over fixture Parquet (30-day window both edges, after AS_OF, distinct multi-line order, NULL buyer, buyer with orders only, missing column exit 2, parity mismatch names `paid_orders_30d`), then `make lint test`

## 2. E2E track (platform-e2e; team-analytics and platform-featurestore FEATURES.yaml)

- [x] 2.1 One FEATURES.yaml acceptance line per new/modified scenario, `planned` with note `needs rebuild`; verify `features-check`
- [x] 2.2 `analytics/order_facts_buyer.feature` (+ `ofb_` steps and flow): a real paid order, the export holds `buyer_id`; verify lint clean and collection
- [x] 2.3 `featurestore/order_features.feature`: paid order becomes a feature (real stack); window, multi-line, NULL buyer, tampered count (real job image on synthetic Parquet inputs); verify lint clean and collection
- [x] 2.4 Integrator: bump `user_activity@v1` -> `@v2` and `current` "1" -> "2" in `fsm_steps.py` and `feature_materialization.feature` (modified scenarios keep their names); verify the fsm scenarios green

## 3. Review and verify

- [x] 3.1 Gate: `openspec validate order-facts-buyer --strict`, `spec_sync`, `repo_doctor`; stack rebuilt with the new team-analytics and featurestore images, new scenarios green

## Evidence (2026-10-09)

- Gate after rebuilding identity, order, gateway, team-ai, analytics and frontend (images built from feat/ui-system):
  - parallel lane (`-n 4 -m "not destructive"`, deselecting test_ui_components): 710 passed and 4 xfailed, run twice
    (w2-par-1, w2-par-2). The 4 xfails are UI defects found by other tracks.
  - destructive lane: 75 passed, 3 failed (w2-destr). All 3 failures are in other changes' new tests
    (notification-delivery-hardening and ui-phase-cart-checkout) and are being fixed separately.
- Also `openspec validate --strict`, `spec_sync --strict` (e2e-ready), `features.py --strict` and repo_doctor.
- No proto change was needed: `OrderPaidEvent.buyer_id` already exists, and team-analytics was dropping it.
- Integrator: the fsm e2e moved to `user_activity@v2`. It now uses a Redis DB per xdist worker (4ba91597), because
  concurrent runs overwrote the meta. Both runs at -n 4 gave 18 passed.
- Also on this branch: the analytics consumer commits offsets only when every polled record is written or skipped
  (05a8e072). This fixes the notify-chat-and-shipment offset defect, and a commit that could cover order facts still
  buffered.
