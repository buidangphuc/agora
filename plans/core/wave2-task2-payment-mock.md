# [W2-T2] Payment mock (team-payment)

## Role
SE

## Objective
Service thanh toán GIẢ LẬP: chọn method → callback "paid" → báo order commit + PAID. Tuyệt đối không tiền thật.

## Write-set (EXCLUSIVE)
- team-payment (edit — mock payment handler/service/repo + migration + vendored proto)

## Read-only dependencies
- platform-core/packages/proto; gRPC client tới team-order (advance) hoặc emit event theo contract
- 00-spec.md §Architecture (payment mock — Rule financially sensitive)

## Contracts you implement
- InitPayment(order_id, method)→{payment_id, status=PENDING}
- SimulateCallback(payment_id, result)→ advance order sang PAID (result=success) / giữ nguyên (fail)

## Acceptance criteria
- [ ] Không có bất kỳ tích hợp cổng tiền thật nào (chỉ mock/in-memory state)
- [ ] Callback success → order PAID (+ commit stock qua luồng order); fail → order không đổi
- [ ] ≥3 test (init, callback-success, callback-fail)

## Verify
docker run go test ./... trong team-payment

## Out of scope
- Không ví/boost; không route gateway (wave3); không sửa team-order nội bộ (gọi qua contract)
