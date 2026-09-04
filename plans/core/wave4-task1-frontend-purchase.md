# [W4-T1] Frontend luồng mua (team-frontend)

## Role
SE

## Objective
UI purchase: sổ địa chỉ, giỏ hàng, checkout, theo dõi đơn (buyer) + đơn shop + advance (seller).

## Write-set (EXCLUSIVE)
- team-frontend (edit — src/app/{cart,checkout,orders,seller/orders,account/addresses} + gateway client)

## Read-only dependencies
- gateway endpoints (W3-T1); 00-spec.md §Conventions (Connect-ES, server-only gateway module)

## Contracts you consume
- order/cart/address/payment qua gateway

## Acceptance criteria
- [ ] Thêm/sửa/xoá địa chỉ + chọn default; giỏ add/update/remove với subtotal
- [ ] Checkout tạo đơn; buyer thấy đơn mình; seller thấy đơn shop + nút advance
- [ ] `next build` + `tsc --noEmit` sạch

## Verify
next build && tsc --noEmit trong team-frontend

## Out of scope
- Không làm assistant/flash/chat/cockpit (wave11); không real-time UI (chờ edge wave7)
