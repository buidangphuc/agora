# [W1-T3] Cart slice (team-order)

## Role
SE

## Objective
team-order có giỏ hàng per-user (add/update/remove/get) + subtotal sống. Không gọi service khác.

## Write-set (EXCLUSIVE)
- team-order (edit — cart handler/service/repo + migration carts/cart_items + vendored proto)

## Read-only dependencies
- platform-core/packages/proto (Cart RPC — W0-T1)
- 00-spec.md §Contracts, §Conventions

## Contracts you implement
- Cart, CartItem{listing_id, qty, unit_price_snapshot}
- RPC AddToCart/UpdateCartItem/RemoveCartItem/GetCart (scope: buyer)

## Acceptance criteria
- [ ] Giỏ theo user; add/update/remove đúng; subtotal cập nhật khi qty đổi
- [ ] Migration carts/cart_items chạy sạch; RequireScopes(buyer)
- [ ] ≥3 test (add, update-qty→subtotal, remove)

## Verify
docker run go test ./... trong team-order

## Out of scope
- KHÔNG làm checkout/order (wave2-task1) — cùng repo, khác wave; không gọi team-domain; không route gateway
