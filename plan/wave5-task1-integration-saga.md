# [W5-T1] Integration Phase A — saga e2e (SOLO)

## Role
SRE

## Objective
Dựng compose đầy đủ (thêm team-order/payment), chạy e2e luồng mua hàng, regenerate file sinh, cập nhật docs.

## Write-set (EXCLUSIVE)
- platform-core/tools (edit — mở rộng seed-marketplace.sh nếu cần)
- platform-core/docs/ROADMAP.md (edit — Phase 4/5 done)

## Read-only dependencies
- Toàn bộ output wave 0–4 đã merge

## Acceptance criteria
- [ ] `docker compose -p platform-core up` chạy được với order + payment
- [ ] E2E: register → address → add-to-cart → checkout → order PENDING_PAYMENT + stock giảm →
      mock pay → PAID → seller advance SHIPPED → buyer thấy đổi
- [ ] order.events có OrderCreated + OrderStatusChanged (soi kafka-ui :8088)

## Verify
docker compose up + kịch bản e2e mua hàng chạy xanh

## Out of scope
- Không sửa business logic service (chỉ wiring/seed/docs); file generated regenerate, không merge tay
