# [W1-T2] Reserve/Commit/Release stock (team-domain)

## Role
SE

## Objective
team-domain có kho product-level + reservation atomic để checkout gọi qua gRPC. Nền cho cả saga và flash-sale.

## Write-set (EXCLUSIVE)
- team-domain (edit — reserve handler/service/repo + migration stock/stock_reservations + vendored proto)

## Read-only dependencies
- platform-core/packages/proto (ReserveStock/Commit/Release — W0-T1)
- 00-spec.md §Architecture (atomic stock), §Contracts

## Contracts you implement
- cột `stock` trên listing (product-level); bảng `stock_reservations{id,listing_id,qty,state}`
- RPC ReserveStock(items)→{reservationId} (trừ khả dụng **atomic**, row-lock),
  CommitStock(reservationId) idempotent, ReleaseStock(reservationId) hoàn lại

## Acceptance criteria
- [ ] Reserve trừ khả dụng atomic; hết hàng → lỗi rõ (không âm kho)
- [ ] Commit idempotent (gọi 2 lần = 1 hiệu ứng); Release hoàn đúng số đã reserve
- [ ] ≥3 test (reserve-ok, over-reserve-fail, commit-idempotent)

## Verify
docker run go test ./... trong team-domain

## Out of scope
- Không emit ListingStockChanged ở đây (đó là wave8-task1); không variant-level; không route gateway
