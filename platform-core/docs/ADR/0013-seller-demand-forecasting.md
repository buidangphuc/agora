# ADR-0013 — Seller Demand Forecasting & Order Warehouse Integration (platform-forecast)

**Status:** Proposed · **Date:** 2026-09-20 · **Relates to:** ADR-0002, ADR-0005, ADR-0008, ADR-0009, ADR-0011

## Context

The seller cockpit already asks a commercial question of the warehouse: `AnalyticsQueryService.GetRevenueBreakdown` returns `TopSku{revenue, units_sold}` and `GetSellerFunnel` returns `impression→view→add→order`. Both read `tracking_events` and project seller facts out of the open-ended properties bag (`team-analytics/internal/query/repository.go`): `seller_id`, `order_id`, `revenue`, `units`, each `TRY_CAST` out of JSON per row (`internal/query/duckdb.go`).

Nothing writes those keys. `analytics.events` has exactly one producer — the gateway's `/api/track` edge collector, fed by the browser beacon in `team-frontend/src/lib/track.ts`, whose `TrackEventType` is `view | click | add_to_cart | impression`. There is no purchase event type and no server-side producer anywhere. The seller's revenue panel reads columns that are empty outside e2e seeding, and `team-order` emits no Kafka events at all (the topics in use are `listing`, `payment`, `promotion`, `chat`, `analytics`).

So "predict how many units a seller will move" is blocked on a prior question that is itself the larger architectural decision: how do purchase facts enter the warehouse? Forecasting is the forcing function, not the whole change.

Second, the forecasting workload's resource profile — nightly batch, Spark-shaped, artifact-producing, no request surface, deploy cadence tied to model version — is the `platform-recsys` profile exactly, and matches nothing in the `team-*` fleet.

## Decision

1. **Purchase facts are event-carried from `team-order`, never beaconed from the browser.** A new Kafka topic `order.events` (key = `order_id`, wrapped in `platform.events.v1.EventEnvelope`), written through a transactional outbox in `team-order` — the same pattern ADR-0005 and ADR-0009 already established in `team-domain` and `team-payment`. The payload carries line items: `listing_id`, `variant_id`, `seller_id`, `quantity`, `unit_price`, `currency`, `occurred_at`, `status`.
2. **Emit on the `PAID` transition, not on order creation.** An unpaid or abandoned order is not realized demand. A return or cancellation after the emit is corrected by a compensating fact, not by mutating history — the table stays append-only.
3. **`team-analytics` consumes into a second warehouse table `order_facts`, not into `tracking_events`.** Different grain (one row per order line), different trust level (server-emitted and authoritative vs. a best-effort, client-authored beacon), different retention pressure. It keeps the same discipline: its own ordered Schema list and a DuckDB↔BigQuery parity test.
4. **The existing seller queries move onto `order_facts`.** `SellerFunnel` and `RevenueBreakdown` stop projecting money out of a JSON string bag. This is the part that has standalone value even if forecasting is never built.
5. **A new repo, `platform-forecast`.** By the §6 decision rule it is not a `team-*` service: it owns no business tables, defines no proto, is not browser-reachable. It is a platform capability on the `platform-recsys` / `platform-modelserve` precedent, run by a nightly platform-gitops CronJob.
6. **Baseline before model.** Ship seasonal-naive + EWMA(7/28d) quantiles first. It is the accuracy floor every later model must beat on the same backtest; a LightGBM global model ships only if it beats baseline WAPE on holdout.
7. **Forecast a distribution, not a point.** p10/p50/p90 per `(seller_id, listing_id, date)` over a 28-day horizon. A point estimate cannot answer "how much should I stock" — safety stock is a function of spread.
8. **The restock number is deterministic arithmetic, not ML, computed in the serving layer:**
   $$\text{reorder\_point} = p50(\text{lead\_time}) + z \cdot (p90 - p50)$$
   Keeping it out of the model makes changing a service-level target a config change, not a retrain.
9. **Output to Redis under the `platform-recsys` conventions** — keys `fc:{SCHEMA_VERSION}:seller:{seller_id}:{listing_id}`, TTL > run cadence, a `model_version` key echoed by the RPC, stale-generation prune. No Qdrant: there are no vectors here.
10. **Served through `team-analytics` — no new service, no new port.** Add `GetDemandForecast` to `analytics.proto`, a `team-gateway` forwarder, and the panel on `/seller/analytics`. That service already owns the seller-analytics surface and the warehouse read path.
11. **A cold-start floor, always.** A listing with less than $N$ days of history falls back to a category/seller median, mirroring `recs:v1:popular`. The RPC never returns "no forecast".
12. **Backtest metrics land in the warehouse, not in a registry.** A `forecast_runs` row per run (`run_id`, `model_version`, `horizon`, WAPE/MASE/pinball loss, interval coverage) plus OTel gauges (ADR-0004).
13. **Advisory only.** The number is a recommendation shown to a seller. It never auto-creates a purchase order, never touches inventory reservations (ADR-0008), and never moves money (§7 financially-sensitive rule).
14. **Delivered in four independently shippable stages:**
    - **Stage 0**: `order.events` + outbox in `team-order` + `order_facts` in `team-analytics` + seller queries rewritten onto `order_facts`.
    - **Stage 1**: `platform-forecast` baseline + Redis + `GetDemandForecast` + UI.
    - **Stage 2**: LightGBM global model, gated on beating the baseline WAPE.
    - **Stage 3**: Restock / reorder point + drift monitoring.

## Alternatives Rejected

- **Extend the browser beacon with a purchase type.** The cheapest option by far: one enum value and one call on the order-success page. Rejected on three counts — the beacon is best-effort by construction (`sendBeacon`, every error swallowed so telemetry never breaks browsing), so revenue would silently under-count; its payload is client-authored, making revenue and units attacker-controlled on a seller-facing money figure; and it misses any order not completed in a live tab.
- **Keep projecting out of properties, just start populating it server-side.** Avoids a second table and a schema. Rejected: a JSON bag of strings with a per-row `TRY_CAST` is the wrong home for the grain we will aggregate nightly over months, and it merges a spoofable beacon stream with authoritative order lines into one table carrying one trust level.
- **Reuse payment.events.** `team-payment` already emits it and `team-order` already consumes it (ADR-0009), so the topic and relayer exist. Rejected: `PaymentSettled` is keyed to an order total and carries no line items — no per-listing quantity, which is the single field forecasting exists to predict.
- **team-analytics polls team-order over gRPC nightly.** Rule-3 compliant, no outbox needed. Rejected: it reintroduces a synchronous cross-service dependency on a batch path, re-derives by hand the "what changed since?" cursor the event log gives for free, and diverges from the CQRS posture every other read-model follows.
- **A second job inside platform-recsys.** Shares the Spark session, warehouse reader, and `model_version` scaffolding. Rejected: different cadence sensitivity and a different output store shape, and a failing forecast run would share a CronJob and an alert surface with recommendations. One repo, one artifact lifecycle.
- **A team-forecast service.** Rejected by the §6 decision rule — it owns no tables a user mutates and has no lifecycle beyond its batch run.
- **MLflow + MinIO + Postgres registry (the reference system's choice).** Genuinely the right answer once models are compared, promoted, and rolled back. Rejected for now: three stateful components to version one nightly artifact that `model_version` + generation prune already versions. The trigger to revisit is recorded in (12) — more than one concurrently-served model, or a need to roll back to a prior artifact.
- **Ray Tune / Airflow / Flink (the rest of the reference stack).** HPO over a seasonal-naive baseline is premature; the gitops CronJob is our scheduler; and Flink's schema-validation stage duplicates what proto plus the analytics consumer already enforce at the edge.

## Consequences

- `team-order` gains its first producer path: an outbox table and relayer (it has `processed_events.go` for idempotent consumption and nothing for emitting). A new topic `order.events` must be provisioned.
- The seller funnel and revenue queries are rewritten; the properties-projection comments and the `colSKU`/`colRevenue`/`colUnits` expressions go away, and any e2e that seeds `properties.revenue` is reworked.
- Demand truth becomes eventually consistent with the order DB (consumer lag). Irrelevant for a nightly forecast; for the revenue panel it turns an instant read into a near-real-time one, and the lag is an SLO to quantify.
- Forecast quality is bounded by history depth. On the current demo dataset the baseline is the only defensible model — stage 2 should not begin before there is a season of real data, and saying so is part of the decision.
- A return or cancellation after the `PAID` emit overstates demand until its compensating fact lands; stage 0 must settle whether the job filters on terminal status or consumes the correction.
- One more repo, one more CronJob, one more Redis keyspace — but no new port, no new gateway-reachable service, and no breaking proto change.
