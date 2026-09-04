# [W12-T1] Integration + Observability spine (SOLO, cuối)

## Role
SRE

## Objective
Dựng compose đầy đủ (thêm team-notification, obs profile ON), chạy trọn golden demo path, xác minh trace xuyên service, seed demo, cập nhật docs.

## Write-set (EXCLUSIVE)
- platform-core/tools (edit — seed-marketplace.sh: thêm sản phẩm flash-sale, dữ liệu demo)
- platform-core/docs (edit — ROADMAP/AGENTS cập nhật trạng thái showcase)

## Read-only dependencies
- Toàn bộ output Phase A/R/B/C đã merge

## Acceptance criteria
- [ ] compose up đầy đủ + obs profile; team-notification chạy
- [ ] **Golden demo path** e2e: assistant → flash-sale (2 browser đồng bộ) → cart → checkout saga
      (+ test-fail → compensation) → mock pay → chat (+ copilot) → chuông → cockpit (ticker + p95/p99 + Inspect Trace)
- [ ] Jaeger: 1 trace nối frontend→gateway→order→domain→kafka→notification hiển thị
- [ ] Grafana SLO/golden signals sống

## Verify
docker compose up (obs) + chạy golden demo path bằng browser + kiểm Jaeger/Grafana

## Out of scope
- Không sửa business logic service (chỉ wiring/seed/docs); file generated regenerate, không merge tay
