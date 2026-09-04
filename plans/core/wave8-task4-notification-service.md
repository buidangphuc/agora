# [W8-T4] Notification service (team-notification — MỚI)

## Role
SE

## Objective
Repo Go mới: consume order.events + chat.events → notifications; đọc "chuông"; push qua edge. Idempotent theo event id.

## Write-set (EXCLUSIVE)
- team-notification (create — copy shape 1 Go service: config/bootstrap/gRPC transport/interceptors/migration + consumers)

## Read-only dependencies
- proto notification (W6-T1); real-time edge (W7-T1); 1 Go service tham chiếu (vd team-engagement) để copy shape
- 00-spec.md §Contracts, §Conventions

## Contracts you implement
- Notification{id,user_id,kind,payload,read,ts}; RPC ListNotifications/MarkRead
- Consumer order.events + chat.events → tạo notification (idempotent theo event id)

## Acceptance criteria
- [ ] Đổi trạng thái đơn / tin nhắn mới → sinh notification cho đúng user
- [ ] **Idempotent**: event trùng id không tạo notification lặp
- [ ] MarkRead hoạt động; push realtime qua edge (room user:{id})
- [ ] ≥4 test (order-noti, chat-noti, duplicate-event, mark-read)

## Verify
docker run go test ./... trong team-notification

## Out of scope
- Không route gateway (wave9); không email/push thật; mirror shape service có sẵn, không tự chế cấu trúc mới
