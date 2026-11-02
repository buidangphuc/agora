## Context

Decisions made by the user on 2026-10-31 (plan `seller-restricted`): inline 403, `status` claim only for restricted, no expiry date, buyer variant without the register button, keep the existing restricted line, two stacked PRs. On 2026-11-02 the user chose to keep `forbiddenPath` in the session config and stop using it in the seller app only.

## Decisions

- The access check stays in `src/app/seller/layout.tsx`, so every `/seller/*` page keeps the requested URL and renders no children for a principal without `listing.write`. Page guards already return `null` for such a principal (`restrictedGuard.test.tsx`).
- The variant is chosen from `principal.status === "restricted"`. A token without the claim (any token signed before PR1, or any non-restricted user) gets the buyer variant. The backend still enforces through scopes, so the claim only picks the text.
- Texts:
  - Buyer: title "Không có quyền", subtitle "Tài khoản của bạn chưa có quyền Người Bán (listing.write).", line "Cần tài khoản Người Bán".
  - Restricted: title "Tài khoản Người Bán đang bị hạn chế", line "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục."
  - Both: link "Về trang mua sắm" to the buyer app, button "Đăng xuất".

## Risks

- An access token issued before a restriction keeps working until it expires (at most one hour); the restriction revokes sessions so the next refresh fails and the user logs in again with the new claim. Unchanged behaviour.
