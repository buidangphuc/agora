## Context

Bulk ship is a seller-app loop over `OrderService.UpdateOrderStatus` (initiative decision); there is no bulk RPC. Single ship already exists as `updateOrderStatusAction` and `ShipOrderButton` (confirm modal with optional tracking number).

## Decisions

- Server action, not client loop: `bulkShipOrdersAction(items: { orderId; trackingNumber? }[])` runs on the server so the token stays server-side and the 4-in-flight limit and 15 s budget are enforced in one place. It validates (principal, `listing.write`, 1..50 distinct non-empty ids) before any upstream call, then runs a small pool of 4 workers over `updateOrderStatus(orderId, SHIPPED, tracking)`. Results keep input order.
- Error mapping (Connect code to text): PERMISSION_DENIED "Không có quyền với đơn này", NOT_FOUND "Không tìm thấy đơn", FAILED_PRECONDITION "Trạng thái đơn không cho phép gửi (có thể đã huỷ hoặc đã gửi)", UNAVAILABLE / DEADLINE_EXCEEDED "Dịch vụ đơn hàng đang bận", anything else "Cập nhật thất bại". Raw server messages are not shown.
- Budget: workers check a deadline (start + 15 s) before taking the next order; remaining orders get result `not_attempted`. In-flight calls are not aborted.
- Selection scope: only `isToShip` rows are selectable; selection is per page (page size is owned by `seller-server-paging-ui`), cap 50 enforced in UI and action. Failed rows stay selected after a batch.
- Print route instead of `window.print()` on the list: the list does not hold full order details (recipient, address) reliably after paging, so the print page re-reads each order with `GetOrder`, which also re-checks ownership. Ids travel in the query string (<= 50 uuids, about 1.9 KB).
- No optimistic UI: rows change only after the revalidated list returns.
