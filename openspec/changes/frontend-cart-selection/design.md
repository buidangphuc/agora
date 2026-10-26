# Design

## Decisions

- Selection is URL state, not stored. The cart island keeps the ticked ids in React state and renders them into the checkout link; `?items=` on `/cart` seeds the initial state (back link). Why: 00-spec "Server-persisted cart selection" is a non-goal and `item_ids` already exists.
- Default is all ticked (human decision C10). Why: it keeps today's one-click "Mua hàng" behaviour for the buyer who wants everything; unticking is the new action.
- One pure helper module (`selection.ts`) shared by the cart island and the checkout page, so "ids not in the cart are ignored, none left means back to the cart" is written once and unit-tested table-driven.
- Absent or empty `items` (`?items=`) means the whole cart (empty `item_ids`), as frontend-checkout-coherence's `checkoutParams` defines; a non-empty list naming no current line means redirect to `/cart`. Why: one rule shared with the sibling slice, and an explicit but dead selection never silently becomes "everything" (00-spec: "`?items=` naming no current cart line sends the buyer back to `/cart`").
- Encoding: every `items` link uses `URLSearchParams` (`items=l1%2Cl3`), the same as `checkoutHref`; parsing decodes it, so both spellings are read.
- The cart no longer carries its `voucher` to checkout: checkout takes scoped vouchers (`sv`/`pv`) and ignores the legacy `voucher` param, so a carried code would be silently dropped. The buyer enters vouchers per scope at checkout.
- `items` is stripped from GTM `page_location` (same place as `returnUrl`), so cart line ids never reach analytics.
- "Mua ngay" reads the line id from the `AddToCartResponse` cart (matching listing + variant), no new RPC. `addToCart` already maps the cart; `addToCartAction` returns `{ itemId }`. A missing line is an error, not a fallback to `/checkout`.
- Cart summary shows the sum of already-displayed server line totals for the ticked lines but not discounts when the selection is partial: a cart voucher preview is computed for the whole cart on the server, and `PreviewCheckout` is the only authority for partial selections. No shipping or total is invented on the cart page (as today, `shipping={null}`).
- The cart page stays a server component; only the island is `"use client"` (existing e2e guards the HTML).

## Clarifications

### Round 1 — 2026-10-16

- C10 [default] Cart selection: every line is ticked by default: Should the cart open with every line ticked? → **Yes, all ticked** (human)
- C10 [ui] Cart selection: a partial selection hides the cart discount and carries no voucher: What does the cart summary show for a voucher on a partial selection? → **Hide the voucher row and say "Giảm giá được tính ở bước thanh toán"** (human)
- X-gateway [dependency] Cart selection: checkout uses only the selected ids: Which slice adds the gateway forwarder for `PreviewCheckout`? → **order-multishop-checkout; this slice adds no gateway task** (human)
