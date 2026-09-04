# Plan Index — Killer Showcase Fullstack Marketplace

Single source of truth cho orchestration. Cập nhật cột **Status** sau mỗi sự kiện task.
Status: `todo | running | review | done | failed`.

## Waves & tasks

| Wave | Task file | Repo (writes) | Verify | Status |
|---|---|---|---|---|
| 0 | wave0-task1-proto-infra-saga | platform-core proto+infra | buf lint/breaking; compose config | done |
| 1 | wave1-task1-identity-addresses | team-identity | go test ./... (Docker) | done |
| 1 | wave1-task2-domain-stock-reserve | team-domain | go test ./... (Docker) | done |
| 1 | wave1-task3-order-cart | team-order | go test ./... (Docker) | done |
| 2 | wave2-task1-order-checkout | team-order | go test ./... (Docker) | done |
| 2 | wave2-task2-payment-mock | team-payment | go test ./... (Docker) | done |
| 3 | wave3-task1-gateway-saga-forward | team-gateway | go test ./... (Docker) | done |
| 4 | wave4-task1-frontend-purchase | team-frontend | next build && tsc | done |
| 5 | wave5-task1-integration-saga | infra+docs | e2e mua hàng | done |
| 6 | wave6-task1-proto-showcase | platform-core proto | buf lint/breaking | done |
| 7 | wave7-task1-gateway-realtime-edge | team-gateway | go test + smoke SSE | done |
| 7 | wave7-task2-infra-observability | platform-core infra | compose up obs; trace visible | done |
| 8 | wave8-task1-domain-flashsale-events | team-domain | go test ./... | done |
| 8 | wave8-task2-ai-endpoints | team-ai | py_compile | done |
| 8 | wave8-task3-chat-service | team-chat | go test ./... | done |
| 8 | wave8-task4-notification-service | team-notification | go test ./... | done |
| 8 | wave8-task5-order-saga-state | team-order | go test ./... | done |
| 9 | wave9-task1-gateway-showcase-routes | team-gateway | go test + smoke | done |
| 10 | wave10-task1-frontend-plumbing | team-frontend (shared) | next build && tsc | done |
| 11 | wave11-task1-fe-assistant | team-frontend/src/app/assistant | next build && tsc | done |
| 11 | wave11-task2-fe-flashsale-meter | team-frontend flash-sale | next build && tsc | done |
| 11 | wave11-task3-fe-saga-timeline | team-frontend/src/app/checkout | next build && tsc | done |
| 11 | wave11-task4-fe-chat-bell | team-frontend chat+notifications | next build && tsc | done |
| 11 | wave11-task5-fe-cockpit | team-frontend/src/app/admin/cockpit | next build && tsc | done |
| 11 | wave11-task6-fe-magic-listing | team-frontend/src/app/seller/new | next build && tsc | done |
| 12 | wave12-task1-integration-showcase | infra+docs | golden demo e2e + Jaeger trace | done |

## Dependency notes
- **Phase A (0→5)** là foundation saga, chạy TRƯỚC toàn bộ showcase. Đẻ `order.events` + dữ liệu đơn/stock.
- **Phase R (6→7)** enabler dùng chung: proto showcase + real-time edge + observability wiring.
- **Phase B (8)** 5 service song song (repo rời). B-saga-state (wave8-task5) cần checkout của wave2.
- **Phase C (9→11)** gateway (solo) → frontend plumbing (solo) → 6 route song song (thư mục rời).
- **Phase D (12)** integration + trace spine + seed + docs.

## Wave 11 note (ceiling)
6 task song song > ngưỡng ~5 — **chấp nhận có chủ đích**: đây là boilerplate-mode (mỗi task = 1
thư mục route rời trong team-frontend), xung đột file bất khả vì write-set disjoint. Điều kiện:
plumbing chung (wave10) phải xong trước để không task nào phải sửa nav/gateway-client/types.

## Merge order
Theo thứ tự wave; trong wave, thứ tự tuỳ ý vì write-set disjoint. Wave N+1 nhánh từ trạng thái
đã-merge của wave N (không từ main cũ). File sinh tự động (buf/codegen, lockfile) KHÔNG nằm trong
write-set nào → regenerate ở wave integration, không merge.

## Fan-out mode
Mode A (subagents). Main session làm mọi wave solo (0,5,6,7-obs,9,10,12); mỗi wave song song
spawn 1 subagent/task. Context subagent = `00-spec.md` + đúng packet + work-rules. Audit sau mỗi
task: `python scripts/validate_plan.py plan/ --audit plan/<packet>.md --base <wave-base>`.
