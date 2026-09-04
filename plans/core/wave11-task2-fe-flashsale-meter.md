# [W11-T2] FE — Flash-sale live meter (team-frontend)

## Role
SE

## Objective
Trên trang sản phẩm flash-sale: thanh `🔥 ĐÃ BÁN xx%` + số tồn kho **tự nhảy real-time** qua SSE khi buyer khác mua (không F5). Optimistic UI khi tự mua.

## Write-set (EXCLUSIVE)
- team-frontend/src/components/flash-sale (create)
- team-frontend/src/app/listing (edit — gắn meter vào trang chi tiết sản phẩm)

## Read-only dependencies
- plumbing (W10-T1: useSse/useRoom room listing:{id}); domain ListingStockChanged (W8-T1) qua edge

## Acceptance criteria
- [ ] Subscribe room listing:{id}; số sold/total cập nhật mượt khi có event (2 browser thấy đồng bộ)
- [ ] Optimistic UI khi chính mình bấm mua; reconcile khi event thật về
- [ ] `next build` + `tsc` sạch

## Verify
next build && tsc --noEmit ; smoke 2 tab thấy số nhảy

## Out of scope
- Không sửa plumbing; không đụng checkout/assistant/chat/admin/seller
