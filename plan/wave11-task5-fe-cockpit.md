# [W11-T5] FE — Live Ops HUD /admin/cockpit (team-frontend)

## Role
SRE

## Objective
Trang HUD dark cho Admin/SRE: Live Order Ticker (SSE ops:orders), Service Health Radar (RPS + p95/p99 từ Prometheus proxy), nút Inspect Trace deep-link sang Jaeger :16686.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/admin/cockpit (create)

## Read-only dependencies
- plumbing (W10-T1: useSse room ops:orders); Prometheus proxy + trace qua gateway (W9-T1)

## Acceptance criteria
- [ ] Order ticker chảy realtime (room ops:orders)
- [ ] Radar đọc RPS + p95/p99 từ **gateway Prometheus proxy** (không gọi thẳng Prom từ browser)
- [ ] Mỗi đơn có nút "Inspect Trace" mở đúng trace trên Jaeger UI
- [ ] `next build` + `tsc` sạch

## Verify
next build && tsc --noEmit

## Out of scope
- Không sửa plumbing; không đụng route khác; không nhúng secret/token Prometheus vào client
