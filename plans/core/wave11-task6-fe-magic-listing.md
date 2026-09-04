# [W11-T6] FE — AI Magic Listing (team-frontend)

## Role
SE

## Objective
Trang đăng tin seller: tải 1 ảnh + gõ tên ngắn → bấm "✨ AI Tạo Mô Tả & Gợi Ý Giá" → form tự điền mô tả SEO + gợi ý category + khoảng giá.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/seller/new (edit/create — nút magic + fill form)

## Read-only dependencies
- plumbing (W10-T1); magic-listing endpoint qua gateway (W9-T1 + W8-T2)

## Acceptance criteria
- [ ] Bấm magic gọi MagicListing → điền description/suggested_price/category vào form
- [ ] Sửa được sau khi AI điền; graceful khi AI trả stub/thiếu field
- [ ] `next build` + `tsc` sạch

## Verify
next build && tsc --noEmit

## Out of scope
- Không sửa plumbing; không đụng route khác; không làm upload ảnh thật (dùng key/placeholder nếu chưa có)
