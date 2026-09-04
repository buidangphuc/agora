# [W11-T4] FE — Chat inbox + AI Copilot + notification bell (team-frontend)

## Role
SE

## Objective
Inbox chat realtime buyer↔seller (seller có **AI Copilot** gợi ý trả lời 1-click) + chuông notification đếm chưa đọc realtime.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/chat (create)
- team-frontend/src/app/notifications (create)
- team-frontend/src/components/notifications (create — bell component)

## Read-only dependencies
- plumbing (W10-T1: useSse room chat:{thread}, user:{id}); chat/notification/copilot qua gateway (W9-T1)

## Acceptance criteria
- [ ] Inbox realtime (room chat:{thread}); lịch sử load; gửi được
- [ ] Seller thấy gợi ý AI Copilot, bấm 1-click để gửi
- [ ] Bell đếm chưa đọc cập nhật realtime (room user:{id}); mark-read hoạt động
- [ ] `next build` + `tsc` sạch

## Verify
next build && tsc --noEmit

## Out of scope
- Không sửa plumbing; không đụng assistant/checkout/flash/admin/seller
