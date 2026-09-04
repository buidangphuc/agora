# [W8-T1] Flash-sale stock events (team-domain)

## Role
SE

## Objective
Mỗi lần stock đổi, team-domain bắn `ListingStockChanged` lên Kafka để edge đẩy về UI real-time.
Thêm cờ flash-sale + sold/total. + OTel spans cho phần này.

## Write-set (EXCLUSIVE)
- team-domain (edit — emit ListingStockChanged trong reserve/commit/release + migration is_flash_sale/sold/total + spans)

## Read-only dependencies
- team-domain reserve slice (W1-T2, đã merge); proto ListingStockChanged (W6-T1)

## Contracts you implement
- Kafka ListingStockChanged{listing_id, sold, total} (key = listing_id, room listing:{id})
- cột is_flash_sale, sold, total trên listing

## Acceptance criteria
- [ ] Mua/reserve/release → event bắn ra Kafka với sold/total đúng
- [ ] Idempotent với reserve đã tính; OTel span bao phủ emit
- [ ] ≥2 test (emit-on-reserve, sold/total-đúng)

## Verify
docker run go test ./... trong team-domain

## Out of scope
- Không làm SSE (đó là edge wave7-task1); không sửa reserve logic lõi (đã có W1-T2)
