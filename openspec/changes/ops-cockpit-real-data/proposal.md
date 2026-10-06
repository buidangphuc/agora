## Why

The admin cockpit (`/admin/cockpit`, `GET /api/admin/metrics`) showed fabricated figures
(1,420 orders, 384,500,000 ₫ GMV, fake order rows and trace ids). A first fix replaced
them with honest empty states, but three gaps remain: the endpoint and page are open to
anyone, orders/GMV/live orders have no real source, and traces are not fetched. The
`ops-cockpit` main spec was also edited directly instead of through a change; this change
restores that and records the new behaviour properly.

## What Changes

- Admin gate: `/api/admin/metrics` requires the `admin` scope (401 / 403); the page checks
  the session server-side and fetches the metrics server-side with the session token.
- `team-analytics` gains admin-scoped `GetPlatformOrderSummary` and `ListRecentOrders`
  RPCs over `order_facts` (additive proto in platform-core).
- The gateway cockpit handler fills `total_orders_24h`, `total_revenue_24h` and
  `recent_orders` from those RPCs, and `recent_traces` from a fixed Jaeger query.
- The HUD renders the real values, or "Chưa có dữ liệu" when a source is unavailable.
- The direct edit to `openspec/specs/ops-cockpit/spec.md` is reverted; this delta carries
  it.

Repos: platform-core (proto), team-analytics, team-gateway, team-frontend, platform-e2e.

## Capabilities

### Modified Capabilities
- `ops-cockpit`: real-data sources, admin gate, never-fabricate rule.

## Non-goals

- No new Prometheus business metric in team-order; orders come from `order_facts`.
- No live-push SSE bridge from Kafka in the gateway; the HUD refreshes on its existing
  poll interval.
- No change to how service error rates are counted (declined reservations still count).

## Impact

- Proto: `platform/analytics/v1/analytics.proto` (additive RPCs and messages), vendored to
  team-analytics, team-gateway.
- Gateway config: `JAEGER_QUERY_URL`, `ANALYTICS_ADDR` (if not already present).
- The e2e cockpit scenarios log in as the seeded admin.
