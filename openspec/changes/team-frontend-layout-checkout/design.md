## Context

The checkout route group has its own distraction-free shell, so it can adopt the platform look without touching the consumer layout. Other agents rebuild the other buyer pages in parallel, and `OrderSummary` and `MobileSummaryBar` are shared with the cart page, so this change stays inside the checkout route files and the payment feature.

## Decisions

- **Touch targets through Link variants.** Standalone text links (footer, back to cart, pay later) use the `ghost` button variant at `size="sm"`, which carries `min-h-touch` from the platform recipe. Inline links inside sentences stay inline. The logo link is a `NextLink` with `min-h-touch` because it is brand markup, not a facade.
- **Mobile-first action rows.** Result actions use `flex-col` with `fullWidth="mobile"` links and buttons, switching to a wrapped row at `sm`, which removes the cramped two-button wrap at 375 px.
- **Overflow guard.** Grid columns and the shell take `min-w-0` so a long address, voucher code or order id shrinks instead of widening the page.
- **Semantic tokens only.** Spacing uses the default 4 px scale, radius `rounded-2xl`/`rounded-xl` from the local config, colours `surface-*`, `text-*`, `border-*`, `action-*` and `focus-ring`.

## Risks / Trade-offs

- The summary card and the mobile bar keep their current look until the cart change touches the shared `OrderSummary`.
- Ghost links read as quieter buttons than the old underlined-on-hover text; the copy is unchanged.
- The visual check ran without the gateway, so it covers the shell, the guest redirect and the error and not-found states; the step forms were checked through vitest only.
