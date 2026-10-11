## Context

`team-order` now publishes `OrderPaidEvent` to `order.events`, and `team-analytics` already
ingests it into `order_facts` (spec `order-facts`). The gateway already has a fixed-query
Prometheus client for the cockpit; Jaeger runs in the stack on :16686. The identity `admin`
role carries the `admin` scope (`team-identity/internal/authz/scopes.go`).

## Decisions

1. **Orders/GMV owner is team-analytics, not the gateway.** The gateway must not compute
   business numbers (rule 2), so the aggregate is an analytics RPC over `order_facts`.
   Distinct orders = `COUNT(DISTINCT order_id)`; GMV = `SUM(quantity * unit_price)`, both
   over `paid_at >= now() - window`.
2. **Recent orders are polled, not pushed.** A Kafka→SSE bridge in the gateway would make
   the edge a consumer; the HUD already polls `/api/admin/metrics`, so `recent_orders` is
   part of that response. The unused `ops:orders` SSE wiring in the page is removed.
3. **Admin check at the gateway, and server-side fetch in the frontend.** The session token
   is an httpOnly cookie, so the browser cannot add a bearer; the page (a server
   component) fetches the metrics with the token, and client-side refresh goes through a
   Next route handler that forwards the token. The gateway enforces `admin`; analytics also
   enforces it on the new RPCs (defence in depth, `RequireScopes`).
4. **Jaeger via a fixed query**, same pattern as Prometheus: `JAEGER_QUERY_URL`,
   `/api/traces?service=team-gateway&lookback=1h&limit=5`, 2 s timeout, shaped output only.

## Risks / Trade-offs

- `order_facts` lag (consumer batch interval) delays the 24h figures; the scenario allows
  30 s.
- Admin-only breaks the existing e2e cockpit scenarios until they log in as admin; the
  e2e track updates them.

## Open Questions

None blocking.
