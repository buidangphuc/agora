## Decisions

- The stepper reaches 44 px through `min-h-touch` on the `QuantityPicker` box: `min-height` wins over the `h-8` the facade sets, the stretch alignment carries the buttons and input to the full height, and the shared facade stays untouched.
- On mobile the stepper and line total sit in their own row indented by the checkbox width (`pl-9`), so they line up with the thumbnail instead of wrapping under the checkbox.
- Shop names truncate inside the card header; the link and its accessible name are unchanged.
- The heading text `Giỏ hàng (N sản phẩm)` and the `pb-24` bottom padding (room for the mobile summary bar, asserted by the page test) are kept.
- The skeleton mirrors the new row padding and hides the line-total placeholder below `sm`, matching the stacked mobile row.

## Risks

- Inline text links (shop name, product title) are text-height links, not 44 px targets; making them taller needs a platform link size, which is out of this change's scope.
- The visual check ran without the gateway, so populated rows were verified only through unit tests; the empty, error and login-redirect states were checked in the browser.
