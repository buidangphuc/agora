# [W11-T1] FE — AI Assistant widget (team-frontend)

## Role
SE

## Objective
Widget chat AI ở góc màn: gõ tự nhiên → stream reply + render **product card** bấm được (Xem/Mua) trong khung chat.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/assistant (create)
- team-frontend/src/components/assistant (create)

## Read-only dependencies
- plumbing (W10-T1: gateway client + useSse); assistant route gateway (W9-T1)

## Acceptance criteria
- [ ] Chat gọi assistant endpoint, hiển thị reply streaming
- [ ] product_cards render thành card tương tác (link chi tiết / add-to-cart)
- [ ] `next build` + `tsc` sạch

## Verify
next build && tsc --noEmit

## Out of scope
- Không sửa plumbing (src/lib, src/hooks, nav, types); không đụng route khác
