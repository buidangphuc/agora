# [W8-T5] Saga state + force-fail hook (team-order)

## Role
SE

## Objective
Expose trạng thái 4 bước saga cho UI timeline + hook ép fail (test-only) để demo compensation trả kho.

## Write-set (EXCLUSIVE)
- team-order (edit — GetSagaState handler + ForceFail hook (guard scope/flag) + spans)

## Read-only dependencies
- team-order checkout slice (W2-T1, đã merge); proto GetSagaState/ForceFail (W6-T1)

## Contracts you implement
- GetSagaState(order_id)→{steps[]:(name,status)} — 4 bước: Order/ReserveStock/Payment/Ship
- ForceFail(order_id) — test-only, guard bằng scope admin/flag env → kích compensation

## Acceptance criteria
- [ ] Đọc được timeline 4 bước phản ánh trạng thái thật của order
- [ ] ForceFail → ReleaseStock + order CANCELLED minh bạch (không rò reservation)
- [ ] ForceFail bị chặn khi không có scope/flag; ≥2 test (get-state, force-fail→compensation)

## Verify
docker run go test ./... trong team-order

## Out of scope
- Không dựng UI (wave11-task3); không đổi state machine lõi (đã có W2-T1)
