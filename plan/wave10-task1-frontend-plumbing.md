# [W10-T1] Frontend plumbing dùng chung (team-frontend, SOLO nhỏ)

## Role
SE

## Objective
Đặt sẵn hạ tầng chung để các route showcase chạy song song không đụng nhau: nav, gateway client
methods, SSE hook, types. Xong = wave11 mở khoá.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib (edit/create — gateway client methods cho ai/chat/notification/order-saga/prom)
- team-frontend/src/hooks (create — useSse/useRoom hook dùng chung)
- team-frontend/src/components/nav (edit — thêm entry assistant/chat/bell/cockpit/seller)
- team-frontend/src/types (create — shared types showcase)

## Read-only dependencies
- gateway showcase routes (W9-T1); 00-spec.md §Conventions

## Acceptance criteria
- [ ] gateway client methods + useSse hook build được, có type
- [ ] Nav có entry các mục mới; `next build` + `tsc` sạch
- [ ] KHÔNG chứa logic riêng của từng route (chỉ plumbing) → wave11 không cần sửa các file này

## Verify
next build && tsc --noEmit trong team-frontend

## Out of scope
- Không dựng nội dung route cụ thể (wave11); không đụng src/app/{assistant,chat,checkout,admin,seller,listing,notifications}
