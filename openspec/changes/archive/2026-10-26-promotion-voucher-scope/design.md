# Design

## Clarifications

### Round 1 — 2026-10-16

- M5 [money/risk] Shop voucher reports SHOP scope and its seller: Is the legacy `CreateOrder.voucher_code` auto-routed to a shop group by probing promotion's echoed scope? → **No. The legacy `voucher_code` is treated as the platform-voucher slot only, with no SHOP auto-routing probe (owner: order-multishop-checkout). This slice still echoes `scope` and `seller_id` on every outcome, so team-order can refuse a SHOP code in the platform slot.** (human)
