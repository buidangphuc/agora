# [W9-T1] Gateway routes cho showcase (team-gateway, SOLO)

## Role
SE

## Objective
Route assistant(streaming)/magic-listing/copilot/chat/notification/saga-state; Prometheus proxy cho cockpit; scope gating.

## Write-set (EXCLUSIVE)
- team-gateway (edit — forwarder ai/chat/notification/saga-state + Prometheus proxy handler + env UPSTREAM_{AI,CHAT,NOTIFICATION}_ADDR)

## Read-only dependencies
- proto showcase (W6-T1); real-time edge (W7-T1, đã merge); 00-spec.md §Architecture (cockpit proxy Prom)

## Contracts you consume
- ai/chat/notification/order-saga-state RPC; Prometheus :9090 (server-side only)

## Acceptance criteria
- [ ] Mọi RPC/stream showcase đi qua gateway; assistant streaming giữ được
- [ ] Prometheus **chỉ** truy cập server-side qua proxy (browser không gọi thẳng)
- [ ] Scope đúng (copilot/saga-force-fail chặn buyer thường); .env.example cập nhật

## Verify
docker run go test ./... + smoke assistant stream + prom proxy

## Out of scope
- Không đụng edge hub nội bộ (W7-T1); không làm UI (wave10/11)
