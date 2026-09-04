# [W11-T3] FE — Saga timeline + test-fail (team-frontend)

## Role
SE

## Objective
Màn checkout hiển thị timeline 4 bước saga (Order→ReserveStock→Payment→Ship) + nút `[Test: Payment fail]` để xem compensation trả kho + order CANCELLED live.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/checkout (edit/create — timeline component + force-fail toggle)

## Read-only dependencies
- plumbing (W10-T1); order GetSagaState/ForceFail qua gateway (W9-T1 + W8-T5)

## Acceptance criteria
- [ ] Timeline phản ánh trạng thái thật qua GetSagaState (poll hoặc SSE)
- [ ] Nút test-fail gọi ForceFail → UI thấy step Payment đỏ → stock trả lại → order CANCELLED
- [ ] Nút test-fail chỉ hiện ở chế độ demo (flag); `next build` + `tsc` sạch

## Verify
next build && tsc --noEmit

## Out of scope
- Không sửa plumbing; không đụng route khác; không đổi logic saga backend
