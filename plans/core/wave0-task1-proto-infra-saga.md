# [W0-T1] Proto + infra cho purchase saga

## Role
SA

## Objective
Định nghĩa toàn bộ contract additive cho saga (order/cart/address/reserve-stock + `order.events`)
và đưa `team-order` (:50055) + `postgres-order` vào compose. Xong = mọi service Phase A code theo hợp đồng này.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto (edit — thêm order.proto, mở rộng identity.proto + domain.proto)
- platform-core/infra (edit — compose thêm team-order + postgres-order)
- platform-core/docs/ROADMAP.md (edit — đánh dấu Phase 4/5 in-progress)

## Read-only dependencies
- platform-core/docs/ADR (0001 proto distribution, 0002 broker, 0005 read-model)
- AGENTS.md §4,§6,§7

## Contracts you implement (xem 00-spec §Contracts — inline bản rút gọn)
- order: Cart/CartItem/Order/OrderItem; OrderStatus enum; RPC AddToCart/UpdateCartItem/
  RemoveCartItem/GetCart/Checkout/GetOrder/ListBuyerOrders/ListShopOrders/AdvanceOrderStatus;
  Kafka order.events{OrderCreated,OrderStatusChanged}.
- identity: Address + Add/Update/Delete/List/SetDefault.
- domain: ReserveStock/CommitStock/ReleaseStock (reservationId), product-level stock.

## Acceptance criteria
- [ ] `buf lint` + `buf breaking` pass (additive; không renumber/remove)
- [ ] compose có team-order(:50055) + postgres-order(:5439) trên network platform-core_default
- [ ] Ghi chú re-vendor `proto/` cho mỗi repo sở hữu (không generate hộ repo khác)

## Verify
docker run buf lint && buf breaking ; docker compose -p platform-core config

## Out of scope
- Không implement handler ở bất kỳ service nào (chỉ contract + infra)
- Không sửa code sinh tự động; không thêm RPC của Phase R (showcase) ở đây
