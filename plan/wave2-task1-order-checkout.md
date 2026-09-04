# [W2-T1] Checkout → Order + state machine (team-order)

## Role
SE

## Objective
Checkout tạo order từ cart, reserve stock qua gRPC team-domain, chạy state machine, emit order.events;
buyer/seller xem đơn, seller advance trạng thái.

## Write-set (EXCLUSIVE)
- team-order (edit — order handler/service/repo + migration orders/order_items + Kafka producer)

## Read-only dependencies
- team-order cart slice (W1-T3, đã merge); platform-core/packages/proto
- gRPC client tới team-domain (ReserveStock/Commit/Release — W1-T2)

## Contracts you implement
- Order/OrderItem; OrderStatus{PENDING_PAYMENT,PAID,SHIPPED,COMPLETED,CANCELLED}
- Checkout(cart)→order (gọi domain.ReserveStock, làm rỗng cart); GetOrder/ListBuyerOrders/
  ListShopOrders/AdvanceOrderStatus; Kafka order.events{OrderCreated,OrderStatusChanged}

## Acceptance criteria
- [ ] Checkout gọi ReserveStock; transition không hợp lệ bị chặn
- [ ] CANCEL → gọi ReleaseStock; emit đúng event lên order.events
- [ ] Isolation: buyer chỉ thấy đơn mình, seller thấy đơn shop
- [ ] ≥4 test (checkout-ok, invalid-transition, cancel→release, buyer/seller-isolation)

## Verify
docker run go test ./... trong team-order

## Out of scope
- Payment (wave2-task2, team-payment); GetSagaState/ForceFail (wave8-task5); không route gateway
