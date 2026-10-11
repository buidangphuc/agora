## 1. Contract — platform-core

- [x] 1.1 Add `GetPlatformOrderSummary` and `ListRecentOrders` (+ messages) to `platform/analytics/v1/analytics.proto`, additive only; verify `buf lint` and `buf breaking` against feat/ui-system pass, then vendor byte-identical copies to team-analytics and team-gateway and regenerate

## 2. Code — team-analytics

- [x] 2.1 Implement both RPCs over `order_facts` with `RequireScopes(ctx, "admin")`; verify unit tests for counts, GMV, window edge, empty table and a non-admin principal (PermissionDenied) pass with `go test ./...`

## 3. Code — team-gateway

- [x] 3.1 Require the `admin` scope on `GET /api/admin/metrics` (401 without a token, 403 without `admin`, no upstream call); verify gateway tests for anonymous, buyer and admin
- [x] 3.2 Fill `total_orders_24h`, `total_revenue_24h`, `recent_orders` from team-analytics, `null`/empty when unavailable; verify tests with a fake analytics client (ok and unavailable)
- [x] 3.3 Add the fixed Jaeger query (`JAEGER_QUERY_URL`, timeout 2 s) for `recent_traces`, empty when unavailable; verify tests with an httptest Jaeger (ok, down); update `.env.example` and compose env

## 4. Code — team-frontend

- [x] 4.1 `/admin/cockpit`: server-side session check (anonymous → `/login`, non-admin → 403 `Result`) and server-side fetch with the token; client refresh through a route handler that forwards the token; verify Vitest for the three cases and `npm run check && npx next build`
- [x] 4.2 Render orders, GMV, recent orders and traces from the response, "Chưa có dữ liệu" when null/empty; remove the unused `ops:orders` SSE wiring; verify Vitest that no hard-coded value renders

## 5. E2E — platform-e2e

- [x] 5.1 Cockpit scenarios log in as the seeded admin; add anonymous-401, buyer-403 and "paid order shows in the 24h figures" scenarios with FEATURES.yaml entries; verify they pass against the agora stack and flip to `automated`
- [x] 5.2 Run `openspec validate ops-cockpit-real-data --strict`; verify it is valid

## Evidence (2026-10-09)

- Gate on feat/ui-system, 2026-10-09: parallel lane 710 passed twice (w2-par-1/2). Destructive lane w2-destr: 75 passed; its 3 failures were fixed and rerun green.
- c1 e2e: 10/10, including the Analytics and Jaeger outage scenarios in the serial lane.
