## 1. Code track

- [ ] 1.1 team-analytics: `buyer_id` in `OrderFactRecord`/`OrderFactsSchema` (D1), consumer maps it (empty -> NULL), DuckDB writer + in-place migration, BigQuery writer + `evolveSchema` for `order_facts` (D3); verify tests that fail without the change (consumer maps the buyer and empty -> NULL; DuckDB round trip and migration of a table without the column keeps old rows with NULL; Parquet export has the column; schema parity), then `make check`
- [ ] 1.2 platform-featurestore: `user_activity@v2` with `paid_orders_30d` (D5), v1 retired, lock regenerated, `buyer_id` required in the orders input (D6), README; verify pytest over fixture Parquet (30-day window both edges, after AS_OF, distinct multi-line order, NULL buyer, buyer with orders only, missing column exit 2, parity mismatch names `paid_orders_30d`), then `make lint test`

## 2. E2E track (platform-e2e; team-analytics and platform-featurestore FEATURES.yaml)

- [ ] 2.1 One FEATURES.yaml acceptance line per new/modified scenario, `planned` with note `needs rebuild`; verify `features-check`
- [ ] 2.2 `analytics/order_facts_buyer.feature` (+ `ofb_` steps and flow): a real paid order, the export holds `buyer_id`; verify lint clean and collection
- [ ] 2.3 `featurestore/order_features.feature`: paid order becomes a feature (real stack); window, multi-line, NULL buyer, tampered count (real job image on synthetic Parquet inputs); verify lint clean and collection
- [ ] 2.4 Integrator: bump `user_activity@v1` -> `@v2` and `current` "1" -> "2" in `fsm_steps.py` and `feature_materialization.feature` (modified scenarios keep their names); verify the fsm scenarios green

## 3. Review and verify

- [ ] 3.1 Gate: `openspec validate order-facts-buyer --strict`, `spec_sync`, `repo_doctor`; stack rebuilt with the new team-analytics and featurestore images, new scenarios green
