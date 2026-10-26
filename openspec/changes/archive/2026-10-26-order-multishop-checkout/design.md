# Design

## Error codes per RPC

- Malformed voucher requests (a second `shop_vouchers` entry for one `seller_id`, one code named twice, `voucher_code` together with `platform_voucher_code`) fail `INVALID_ARGUMENT` in both `PreviewCheckout` and `CreateOrder`, before any promotion call.
- A refused voucher (promotion refusal, unknown code, wrong echoed scope or seller, a seller with no group in the selection) is reported `valid=false` with the reason in `PreviewCheckout` (totals quoted without it) and aborts `CreateOrder` with `FAILED_PRECONDITION` and the reason, no order and no hold left.
- Without a promotion client, preview reports every voucher `valid=false` with the fixed reason "vouchers unavailable"; `CreateOrder` ignores vouchers (as today).
- Identity unreachable fails `PreviewCheckout` `UNAVAILABLE`, the same fail-closed rule as `CreateOrder` (author choice from spec-quality-reviewer round 1).
- The requirement title says "reserves nothing" instead of "read-only": the q5 price pre-check still re-prices the buyer's own changed cart lines.

## Clarifications

### Round 1 — 2026-10-16

- M5 [money] Multishop checkout: A legacy shop code is refused as a platform voucher: How is the legacy `CreateOrder.voucher_code` routed? → **Platform-voucher slot only; no SHOP auto-routing probe** (human)
- C8 [contract] Multishop checkout: A legacy platform code becomes the platform voucher: Which enum carries `VoucherQuote.scope`? → **A local `CheckoutVoucherScope` enum in order.proto (no import of promotion.proto, no extra proto copies); team-order maps promotion's scope to it** (human)
- C9 [error] Multishop checkout: Preview with addresses but no default is refused: What happens for a buyer with addresses but no default and an empty `address_id`? → **FAILED_PRECONDITION "address not found" (no first-address fallback)** (human)
- X1 [ownership] Multishop checkout: The gateway forwards PreviewCheckout with the caller's identity: Which slice owns the gateway forwarder for PreviewCheckout? → **order-multishop-checkout** (human)
