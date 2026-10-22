# Design

## Clarifications

### Round 1 — 2026-10-16

- M1 [money] Checkout coherence: the pay page creates a missing transaction: Can a transaction created on the pay page settle after the order was cancelled? → **No; ProcessMockPayment re-reads the order and refuses a CANCELLED one (owned by payment-unpaid-cancel); recorded in the threat model** (human)
- M5 [money] Checkout coherence: only valid vouchers are placed: How is the legacy CreateOrder.voucher_code treated? → **As the platform-voucher slot only, no SHOP auto-routing; this slice never sends it** (human)
- C9 [error] Checkout coherence: a missing address is explained: What happens for a buyer with addresses but no default and an empty address_id? → **team-order answers FAILED_PRECONDITION "address not found" (no first-address fallback); the frontend shows the fixed "address not found" message with the link to the address step** (human)
- X1 [dependency] Checkout coherence: two shops show two groups with their own shipping: Which slice adds the gateway forwarder for PreviewCheckout? → **order-multishop-checkout; this slice adds no gateway code** (human)
